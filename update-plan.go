package ocfl

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"path"
	"runtime"
	"slices"

	ocflfs "github.com/srerickson/ocfl-go/fs"
	"golang.org/x/sync/errgroup"
)

// ErrRevertUpdate: can't revert an update because the update ran to completion
var ErrRevertUpdate = errors.New("the update has completed and cannot be reverted")

// UpdatePlan is a sequence of steps ([PlanStep]) for updating an OCFL object.
// It allows updates to be interrupted, resumed, retried or reverted. To update
// an object, each [PlanStep] in the UpdatePlan must run to completion. The
// result of each step (whether the step completed successfuly or resulted in an
// error) is stored so that repeated updates resume where the previous run
// stopped or failed. Each PlanStep includes compensating actions for reverting
// partial updates that cannot be completed.
type UpdatePlan struct {
	steps  PlanSteps
	newInv *StoredInventory
	oldInv *StoredInventory

	// options
	goLimit int
	logger  *slog.Logger
}

// newUpdatePlan builds an *UpdatePlan that be used to update the object at
// objDir in objFS, transitioning from oldInv to newInv, with new content
// available in src.
func newUpdatePlan(newInv *Inventory, oldInv *StoredInventory) (*UpdatePlan, error) {
	newInvBytes, invDigest, err := newInv.marshal()
	if err != nil {
		return nil, fmt.Errorf("building new inventory: %w", err)
	}
	u := &UpdatePlan{
		newInv: &StoredInventory{Inventory: *newInv, digest: invDigest, bytes: newInvBytes},
		oldInv: oldInv,
	}
	if err := u.prepareSteps(); err != nil {
		return nil, err
	}
	return u, nil
}

// Apply runs incomplete steps in the UpdatePlan and returns the object's new
// *StoredInventory if the updated succeeded. If any step in the UpdatePlan
// results in an error, execution stops and the error is returned. Some steps in
// the plan may run concurrently. Use SetGoLimit to set number of goroutines
// used to run concurrent steps.
func (u *UpdatePlan) Apply(ctx context.Context, objFS ocflfs.FS, objDir string, src ContentSource) (*StoredInventory, error) {
	err := runSteps(ctx, u.IncompleteSteps(), objFS, objDir, src, u.goLimit, u.logger, false)
	if err != nil {
		return nil, err
	}
	return u.newInv, nil
}

// BaseInventoryDigest returns the digest of the object's existing inventory.json.
// It returns an empty string if the UpdatePlan would create a new object.
func (u UpdatePlan) BaseInventoryDigest() string {
	if u.oldInv == nil {
		return ""
	}
	return u.oldInv.digest
}

// Completed return true if all u's steps are marked as completed
func (u UpdatePlan) Completed() bool {
	for range u.IncompleteSteps() {
		return false
	}
	return true
}

// CompletedSteps is an iterator over completed steps in reverse order (most
// recently completed first).
func (u UpdatePlan) CompletedSteps() iter.Seq[*PlanStep] {
	return func(yield func(*PlanStep) bool) {
		for i := range slices.Backward(u.steps) {
			step := &u.steps[i]
			if !step.state.Completed {
				continue
			}
			if !yield(step) {
				break
			}
		}
	}
}

// Err returns an error that wraps all errors found in the u's steps.
func (u UpdatePlan) Err() error {
	var errs []error
	for _, step := range u.steps {
		if step.state.Err != "" {
			errs = append(errs, errors.New(step.state.Err))
		}
	}
	return errors.Join(errs...)
}

// Eq returns true if u and other represent that same update plan, with
// the same set of steps.
func (u UpdatePlan) Eq(other *UpdatePlan) bool {
	if !bytes.Equal(u.newInv.bytes, other.newInv.bytes) {
		return false
	}
	if (u.oldInv == nil) != (other.oldInv == nil) {
		return false
	}
	if u.oldInv != nil && !bytes.Equal(u.oldInv.bytes, other.oldInv.bytes) {
		return false
	}
	return u.steps.Eq(other.steps)
}

// IncompleteSteps is an iterator over incomplete steps in u. Incomplete steps
// may have errors.
func (u *UpdatePlan) IncompleteSteps() iter.Seq[*PlanStep] {
	return func(yield func(*PlanStep) bool) {
		for i := range u.steps {
			step := &u.steps[i]
			if step.state.Completed {
				continue
			}
			if !yield(step) {
				break
			}
		}
	}
}

// MarshalBinary returns a binary representation of u
func (u UpdatePlan) MarshalBinary() ([]byte, error) {
	toEncode := updatePlanState{Steps: u.steps}
	if u.newInv != nil {
		toEncode.NewInventoryBytes = u.newInv.bytes
	}
	if u.oldInv != nil {
		toEncode.OldInventoryBytes = u.oldInv.bytes
	}
	var buff bytes.Buffer
	if err := gob.NewEncoder(&buff).Encode(toEncode); err != nil {
		return nil, err
	}
	return buff.Bytes(), nil
}

// ObjectID returns the ID of the OCFL Object that the UpdatePlan must be
// applied to.
func (u *UpdatePlan) ObjectID() string {
	return u.newInv.ID
}

// Revert calls the 'Revert' function on all Completed steps in u's update plan
// unless all steps have been completed. If all steps have been completed Revert
// has no effect and returns an ErrRevertUpdate.
func (u *UpdatePlan) Revert(ctx context.Context, objFS ocflfs.FS, objDir string, src ContentSource) error {
	if u.Completed() {
		return ErrRevertUpdate
	}
	return runSteps(ctx, u.CompletedSteps(), objFS, objDir, src, u.goLimit, u.logger, true)
}

// SetGoLimti sets the number of goroutines used for processing Steps with Async
// == true. The default value is runtime.NumCPU()
func (u *UpdatePlan) setGoLimit(gos int) { u.goLimit = gos }

// setLogger sets a logger that will be used when running stesp in u.
func (u *UpdatePlan) setLogger(logger *slog.Logger) { u.logger = logger }

// Steps iterates over all steps in the update plan
func (u UpdatePlan) Steps() iter.Seq[*PlanStep] {
	return func(yield func(*PlanStep) bool) {
		for i := range u.steps {
			step := &u.steps[i]
			if !yield(step) {
				break
			}
		}
	}
}

// UnmarshalBinary decodes b as a binary representation of an UpdatePlan
// and sets u to match.
func (u *UpdatePlan) UnmarshalBinary(b []byte) error {
	var decoded updatePlanState
	var newInv, oldInv *StoredInventory
	err := gob.NewDecoder(bytes.NewReader(b)).Decode(&decoded)
	if err != nil {
		return err
	}
	newInv, err = newStoredInventory(decoded.NewInventoryBytes)
	if err != nil {
		return err
	}
	if len(decoded.OldInventoryBytes) > 0 {
		oldInv, err = newStoredInventory(decoded.OldInventoryBytes)
		if err != nil {
			return err
		}
	}
	u.newInv = newInv
	u.oldInv = oldInv
	u.steps = decoded.Steps
	if err := u.prepareSteps(); err != nil {
		return err
	}
	return nil
}

// prepareSteps is used to regenerate u's Step functions. This is only needed
// if u was created by unmarshaling from a binary representation.
func (u *UpdatePlan) prepareSteps() error {
	// the unmarshaled newSteps have Done and Err state, but their run functions
	// nil: rebuild the newSteps to run and import the previous run state.
	newSteps, err := newPlanSteps(u.newInv, u.oldInv)
	if err != nil {
		return err
	}
	if u.steps == nil {
		u.steps = newSteps
		return nil
	}
	if !newSteps.Eq(u.steps) {
		return errors.New("previous update log doesn't reflect current update plan")
	}
	for i := range newSteps {
		u.steps[i].run = newSteps[i].run
		u.steps[i].revert = newSteps[i].revert
	}
	return nil
}

func newPlanSteps(newInv, oldInv *StoredInventory) (PlanSteps, error) {
	newFiles := newInv.versionContent(newInv.Head)
	newHead := newInv.Head.String()
	newSpec := newInv.Type.Spec
	newAlg := newInv.DigestAlgorithm
	var oldInvBytes []byte
	var oldInvDigest, oldAlg string
	var oldHead VNum
	var oldSpec Spec
	if oldInv != nil {
		if oldInv.ID != newInv.ID {
			err := fmt.Errorf("new inventory ID does not match previous inventory's: %q != %q", newInv.ID, oldInv.ID)
			return nil, err
		}
		oldInvBytes = oldInv.bytes
		oldInvDigest = oldInv.digest
		oldAlg = oldInv.DigestAlgorithm
		oldHead = oldInv.Head
		oldSpec = oldInv.Type.Spec
	}
	if newSpec.Cmp(oldSpec) < 0 {
		err := fmt.Errorf("new version's OCFL spec (%q) cannot be lower than the previous version's (%q)", newSpec, oldSpec)
		return nil, err
	}
	if newInv.Head.num != oldHead.num+1 {
		return nil, errors.New("new inventory 'head' is not incremented by 1")
	}
	plan := []PlanStep{
		// initial step is noop with revert to remove the entire object root
		// for v1 updates.
		{
			state: planStepState{Name: "object root "},
			run: func(_ context.Context, _ ocflfs.FS, _ string, _ ContentSource) (int64, error) {
				return 0, nil
			},
			revert: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) error {
				// delete entire object root to revert first version.
				if newInv.Head.num == 1 {
					return ocflfs.RemoveAll(ctx, objFS, objDir)
				}
				return nil
			},
		},
	}
	plan = append(plan,
		// steps for object declaration files
		updateDeclarationSteps(newSpec, oldSpec)...,
	)
	plan = append(plan,
		// a step to remove entire version directory during during revert.
		PlanStep{
			state: planStepState{Name: "version directory " + newHead},
			run: func(_ context.Context, _ ocflfs.FS, _ string, _ ContentSource) (int64, error) {
				return 0, nil
			},
			revert: func(ctx context.Context, objFS ocflfs.FS, objDir string, src ContentSource) error {
				// remove everything in the new version directory
				objVerDir := path.Join(objDir, newHead)
				return ocflfs.RemoveAll(ctx, objFS, objVerDir)
			},
		},
	)
	plan = append(plan,
		// steps to copy contents into the version directory
		updateVersionContentsSteps(newFiles)...,
	)
	plan = append(plan,
		// steps to update inventories and sidecars in version directory and roo,t
		updateInventorySteps(
			newInv.bytes, oldInvBytes,
			newInv.digest, oldInvDigest,
			newInv.Head, newAlg, oldAlg,
		)...,
	)

	return plan, nil
}

// PlanSteps is a series of named steps for performating an object update and
// rolling it back if necessary.
type PlanSteps []PlanStep

func (s PlanSteps) Eq(s2 PlanSteps) bool {
	if len(s) != len(s2) {
		return false
	}
	for i := range s {
		this := s[i]
		that := s2[i]
		// two steps are equal if the values set during prepare() are equal.
		if this.state.Name != that.state.Name {
			return false
		}
		if this.state.ContentDigest != that.state.ContentDigest {
			return false
		}
	}
	return true
}

// UndoStep is a single step in an UpdatePlan.
type PlanStep struct {
	// plan state included in binary representation
	state planStepState
	// run step concurrently with other async steps
	async bool
	// run performs the step's actions. it returns an (optional) size for content
	// written to the object and an error.
	run func(ctx context.Context, objFS ocflfs.FS, objDir string, src ContentSource) (int64, error)
	// revert undoes the run step.
	revert func(ctx context.Context, objFS ocflfs.FS, objDir string, src ContentSource) error
}

func (step PlanStep) MarshalBinary() ([]byte, error) {
	var buff bytes.Buffer
	if err := gob.NewEncoder(&buff).Encode(step.state); err != nil {
		return nil, err
	}
	return buff.Bytes(), nil
}

// ErrMsg returns any error message from the step's last Run.
func (step PlanStep) ErrMsg() string { return step.state.Err }

// Completed returns true if the step ran without error or was successfully
// reverted.
func (step PlanStep) Completed() bool { return step.state.Completed }

// ContentDigest returns the digest of the content (if any) copied during
// the step.
func (step PlanStep) ContentDigest() string { return step.state.ContentDigest }

// Name returns the step's unique name
func (step PlanStep) Name() string { return step.state.Name }

// Run runs the step's function if the step is not marked as complete, recording
// any error message to Err. If the step does not return an error, it is marked
// as complete and any previous error message is cleared.
func (step *PlanStep) Run(ctx context.Context, objFS ocflfs.FS, objDir string, src ContentSource) error {
	if step.run == nil {
		return nil
	}
	if step.state.Completed {
		return nil
	}
	size, err := step.run(ctx, objFS, objDir, src)
	if err != nil {
		msg := err.Error()
		if msg == "" {
			msg = "unspecified error"
		}
		step.state.Err = msg
		return err
	}
	step.state.Size = size
	step.state.Completed = true
	step.state.Err = ""
	return nil
}

// Revert calls step's undo function if the step is marked as complete. If the
// undo function returns an error, the error message is saved as RevertErr. If
// Revert does not result in an error, the step is marked as incomplete and
// UndoErr is cleared.
func (step *PlanStep) Revert(ctx context.Context, objFS ocflfs.FS, objDir string, src ContentSource) error {
	if step.revert == nil {
		return nil
	}
	if !step.state.Completed {
		return nil
	}
	err := step.revert(ctx, objFS, objDir, src)
	if err != nil {
		msg := err.Error()
		if msg == "" {
			msg = "unspecified error"
		}
		step.state.RevertErr = msg
		return err
	}
	step.state.Completed = false
	step.state.RevertErr = ""
	return nil
}

// Size returns the number of bytes copied to the object as part of the steps
// run action. This is only set after the step has run
func (step *PlanStep) Size() int64 {
	return step.state.Size
}

func (step *PlanStep) UnmarshalBinary(b []byte) error {
	var state planStepState
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&state); err != nil {
		return err
	}
	step.state = state
	return nil
}

// steps for setting/updating an ocfl object declaration
func updateDeclarationSteps(newSpec, oldSpec Spec) []PlanStep {
	steps := []PlanStep{}
	if newSpec == oldSpec {
		return steps
	}
	newDecl := Namaste{Type: NamasteTypeObject, Version: newSpec}
	steps = append(steps, PlanStep{
		state: planStepState{Name: "write " + newDecl.Name()},
		run: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) (int64, error) {
			return 0, WriteDeclaration(ctx, objFS, objDir, newDecl)
		},
		revert: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) error {
			objDecl := path.Join(objDir, newDecl.Name())
			err := ocflfs.Remove(ctx, objFS, objDecl)
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		},
	})
	if !oldSpec.Empty() {
		oldDecl := Namaste{Type: NamasteTypeObject, Version: oldSpec}
		steps = append(steps, PlanStep{
			state: planStepState{Name: "remove " + oldDecl.Name()},
			run: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) (int64, error) {
				oldObjDecl := path.Join(objDir, oldDecl.Name())
				err := ocflfs.Remove(ctx, objFS, oldObjDecl)
				if errors.Is(err, fs.ErrNotExist) {
					err = nil
				}
				return 0, err
			},
			revert: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) error {
				return WriteDeclaration(ctx, objFS, objDir, oldDecl)
			},
		})
	}
	return steps
}

// steps for copying files into the object's version directory
func updateVersionContentsSteps(newContent PathMap) []PlanStep {
	var steps []PlanStep
	for dstName, dig := range newContent.SortedPaths() {
		steps = append(steps, PlanStep{
			state: planStepState{
				Name:          "copy " + dstName,
				ContentDigest: dig,
			},
			async: true,
			run: func(ctx context.Context, objFS ocflfs.FS, objDir string, src ContentSource) (int64, error) {
				dstPath := path.Join(objDir, dstName)
				srcFS, srcPath := src.GetContent(dig)
				if srcFS == nil {
					return 0, fmt.Errorf("content source doesn't provide %q", dig)
				}
				return ocflfs.Copy(ctx, objFS, dstPath, srcFS, srcPath)
			},
			revert: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) error {
				dstPath := path.Join(objDir, dstName)
				err := ocflfs.Remove(ctx, objFS, dstPath)
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			},
		})
	}
	return steps
}

// steps for updating object's version and root inventories.
func updateInventorySteps(
	newInvBytes []byte, oldInvBytes []byte,
	newInvDigest string, oldInvDigest string,
	newHead VNum, newAlg string, oldAlg string,
) []PlanStep {
	var steps []PlanStep
	invSidecar := inventoryBase + "." + newAlg
	verDir := newHead.String()
	verDirInv := path.Join(verDir, inventoryBase)
	verDirInvSidecar := verDirInv + "." + newAlg
	// write version directory inventory.json
	steps = append(steps, PlanStep{
		state: planStepState{Name: "write " + verDirInv},
		run: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) (int64, error) {
			objVerDirInv := path.Join(objDir, verDirInv)
			return ocflfs.Write(ctx, objFS, objVerDirInv, bytes.NewReader(newInvBytes))
		},
		revert: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) error {
			objVerDirInv := path.Join(objDir, verDirInv)
			err := ocflfs.Remove(ctx, objFS, objVerDirInv)
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		},
	})
	// write version directory inventory sidecar
	steps = append(steps, PlanStep{
		state: planStepState{Name: "write " + verDirInvSidecar},
		run: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) (int64, error) {
			objVerDir := path.Join(objDir, verDir)
			return 0, writeInventorySidecar(ctx, objFS, objVerDir, newInvDigest, newAlg)
		},
		revert: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) error {
			objVerDirInvSidecar := path.Join(objDir, verDirInvSidecar)
			err := ocflfs.Remove(ctx, objFS, objVerDirInvSidecar)
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		},
	})
	// write root inventory.json
	steps = append(steps, PlanStep{
		state: planStepState{Name: "write " + inventoryBase},
		run: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) (int64, error) {
			objInv := path.Join(objDir, inventoryBase)
			return ocflfs.Write(ctx, objFS, objInv, bytes.NewReader(newInvBytes))
		},
		revert: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) error {
			objInv := path.Join(objDir, inventoryBase)
			if newHead.num == 1 {
				err := ocflfs.Remove(ctx, objFS, objInv)
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			_, err := ocflfs.Write(ctx, objFS, objInv, bytes.NewReader(oldInvBytes))
			return err
		},
	})
	// write root inventory sidecar
	steps = append(steps, PlanStep{
		state: planStepState{Name: "write " + invSidecar},
		run: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) (int64, error) {
			err := writeInventorySidecar(ctx, objFS, objDir, newInvDigest, newAlg)
			if err != nil {
				return 0, err
			}
			if oldAlg == "" || oldAlg == newAlg {
				return 0, nil
			}
			// previous sidecar used a different algorithm needs to be removed
			oldInvSidecar := path.Join(objDir, inventoryBase) + "." + oldAlg
			err = ocflfs.Remove(ctx, objFS, oldInvSidecar)
			if errors.Is(err, fs.ErrNotExist) {
				err = nil
			}
			return 0, err
		},
		revert: func(ctx context.Context, objFS ocflfs.FS, objDir string, _ ContentSource) error {
			objInvSidecar := path.Join(objDir, invSidecar)
			if newHead.num == 1 {
				err := ocflfs.Remove(ctx, objFS, objInvSidecar)
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			// replace the new inventory sidecar with the old one (they may be
			// separate files).
			if err := writeInventorySidecar(ctx, objFS, objDir, oldInvDigest, oldAlg); err != nil {
				return err
			}
			if oldAlg == "" || oldAlg == newAlg {
				return nil
			}
			// new sidecar uses a different algorithm and needs to be removed
			err := ocflfs.Remove(ctx, objFS, objInvSidecar)
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		},
	})
	return steps
}

// run steps, forward or backward
func runSteps(
	ctx context.Context,
	steps iter.Seq[*PlanStep],
	objFS ocflfs.FS,
	objDir string,
	src ContentSource,
	gos int,
	logger *slog.Logger,
	backward bool,
) error {
	if gos < 1 {
		gos = runtime.NumCPU()
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	var group *errgroup.Group
	var groupCtx context.Context
	for step := range steps {
		if step.async {
			if group == nil {
				// The group used for consecutive async steps. If any of the
				// consecutive async steps returns an error, the context for all
				// of them is canceled.
				group, groupCtx = errgroup.WithContext(ctx)
				group = &errgroup.Group{}
				group.SetLimit(gos)
			}
			group.Go(func() error {
				var err error
				switch {
				case backward:
					logger.Info("reverting", "step", step.state.Name)
					err = step.Revert(groupCtx, objFS, objDir, src)
					if err != nil {
						logger.Error(err.Error())
					}
				default:
					logger.Info(step.state.Name)
					err = step.Run(groupCtx, objFS, objDir, src)
					if err != nil {
						logger.Error(err.Error())
					}
				}
				return err
			})
			continue
		}
		// Sync step
		// wait for any previous async steps to complete
		if group != nil {
			if err := group.Wait(); err != nil {
				return err
			}
			group = nil
			groupCtx = nil
		}
		switch {
		case backward:
			logger.Info("reverting", "step", step.state.Name)
			if err := step.Revert(ctx, objFS, objDir, src); err != nil {
				logger.Error(err.Error())
				return err
			}
		default:
			logger.Info(step.state.Name)
			if err := step.Run(ctx, objFS, objDir, src); err != nil {
				logger.Error(err.Error())
				return err
			}
		}
	}
	if group != nil {
		return group.Wait()
	}
	return nil
}

type updatePlanState struct {
	NewInventoryBytes []byte
	OldInventoryBytes []byte
	Steps             []PlanStep
}

type planStepState struct {
	// set during prepare
	Name          string
	ContentDigest string

	// set during run
	Err       string
	RevertErr string
	Completed bool
	Size      int64
}
