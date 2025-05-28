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

// UpdatePlan represents steps for updating an OCFL object, creating a new
// object version.
type UpdatePlan struct {
	newInv *StoredInventory
	oldInv *StoredInventory
	steps  PlanSteps

	// options
	goLimit int
	logger  *slog.Logger
}

// NewUpdatePlan builds an *UpdatePlan that be used to update the object at
// objDir in objFS, transitioning from oldInv to newInv, with new content
// available in src.
func NewUpdatePlan(objFS ocflfs.FS, objDir string, newInv *Inventory, oldInv *StoredInventory, src ContentSource) (*UpdatePlan, error) {
	newInvBytes, invDigest, err := newInv.marshal()
	if err != nil {
		return nil, fmt.Errorf("building new inventory: %w", err)
	}
	u := &UpdatePlan{
		newInv: &StoredInventory{Inventory: *newInv, digest: invDigest, bytes: newInvBytes},
		oldInv: oldInv,
	}
	if err := u.BuildSteps(objFS, objDir, src); err != nil {
		return nil, err
	}
	return u, nil
}

// Apply runs u's incomplete steps and returns the *StoredInventory upon
// completion. It stops at the first error and returns the error. Consecutive
// steps with Async == true are run concurrently. Use SetGoLimit to set number
// of goroutines used to run concurrent steps.
func (u *UpdatePlan) Apply(ctx context.Context) (*StoredInventory, error) {
	if err := runSteps(ctx, u.IncompleteSteps(), u.goLimit, u.logger, false); err != nil {
		return nil, err
	}
	return u.newInv, nil
}

// BuildSteps is used to regenerate u's Step functions. This is only needed
// if u was created by unmarshaling from a binary representation.
func (u *UpdatePlan) BuildSteps(objFS ocflfs.FS, objDir string, src ContentSource) error {
	// the unmarshalled newSteps have Done and Err state, but their run functions
	// nil: rebuild the newSteps to run and import the previous run state.
	newSteps, err := newPlanSteps(objFS, objDir, u.newInv, u.oldInv, src)
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

// Revert calls the 'Revert' function on all Completed steps in u's update plan
// unless all steps have been completed. If all steps have been completed Revert
// has no effect and returns an error.
func (u *UpdatePlan) Revert(ctx context.Context) error {
	return runSteps(ctx, u.CompletedSteps(), u.goLimit, u.logger, true)
}

// SetGoLimti sets the number of goroutines used for processing Steps with Async
// == true. The default value is runtime.NumCPU()
func (u *UpdatePlan) SetGoLimit(gos int) { u.goLimit = gos }

// SetLogger sets a logger that will be used when running stesp in u.
func (u *UpdatePlan) SetLogger(logger *slog.Logger) { u.logger = logger }

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
	return nil
}

func newPlanSteps(objFS ocflfs.FS, objDir string, newInv, oldInv *StoredInventory, src ContentSource) (PlanSteps, error) {
	newFiles := newInv.versionContent(newInv.Head)
	newHead := newInv.Head
	newVersionDir := path.Join(objDir, newHead.String())
	newSpec := newInv.Type.Spec
	newAlg := newInv.DigestAlgorithm
	var oldInvBytes []byte
	var oldInvDigest, oldAlg string
	var oldHead VNum
	var oldSpec Spec
	if oldInv != nil {
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
	if newHead.num != oldHead.num+1 {
		return nil, errors.New("new inventory includes more than one new version")
	}
	for _, dig := range newFiles.SortedPaths() {
		if fsys, _ := src.GetContent(dig); fsys == nil {
			return nil, fmt.Errorf("content source doesn't provide %q", dig)
		}
	}
	plan := []PlanStep{
		// initial step is noop with revert to remove the entire object root
		// for v1 updates.
		{
			state: planStepState{Name: "object root " + objDir},
			run:   func(ctx context.Context) (int64, error) { return 0, nil },
			revert: func(ctx context.Context) error {
				// delete entire object root to revert first version.
				if newHead.num == 1 {
					return ocflfs.RemoveAll(ctx, objFS, objDir)
				}
				return nil
			},
		},
	}
	plan = append(plan,
		// steps for object declaration files
		updateDeclarationSteps(objFS, objDir, newSpec, oldSpec)...,
	)
	plan = append(plan,
		// a step to remove entire version directory during during revert.
		PlanStep{
			state: planStepState{Name: "version directory " + newVersionDir},
			run:   func(ctx context.Context) (int64, error) { return 0, nil },
			revert: func(ctx context.Context) error {
				// remove everything in the new version directory
				return ocflfs.RemoveAll(ctx, objFS, newVersionDir)
			},
		},
	)
	plan = append(plan,
		// steps to copy contents into the version directory
		updateVersionContentsSteps(objFS, objDir, newFiles, src)...,
	)
	plan = append(plan,
		// steps to update inventories and sidecars in version directory and roo,t
		updateInventoriesSteps(
			objFS, objDir, newHead,
			newInv.bytes, oldInvBytes,
			newInv.digest, oldInvDigest,
			newAlg, oldAlg,
		)...,
	)
	plan = append(plan,
		// final step is used to prever the plan from being reverting
		// if the plan completed
		PlanStep{
			state: planStepState{Name: "update complete"},
			run:   func(ctx context.Context) (int64, error) { return 0, nil },
			revert: func(_ context.Context) error {
				return errors.New("the update is complete and cannot be reverted")
			},
		},
	)

	return plan, nil
}

// PlanSteps is a series of named steps for performating an object update and
// rolling it back if necessary.
type PlanSteps []PlanStep

func (s PlanSteps) Eq(other PlanSteps) bool {
	if len(s) != len(other) {
		return false
	}
	for i := range s {
		if s[i].state.Name != other[i].state.Name {
			return false
		}
	}
	return true
}

// UndoStep is a single step in an UpdatePlan.
type PlanStep struct {
	// plan state included in binary representation
	state planStepState
	// run performs the step's actions. it returns an (optional) size for content
	// written to the object and an error.
	run func(ctx context.Context) (int64, error)
	// revert undoes the run step.
	revert func(ctx context.Context) error
}

func (step PlanStep) MarshalBinary() ([]byte, error) {
	var buff bytes.Buffer
	if err := gob.NewEncoder(&buff).Encode(step.state); err != nil {
		return nil, err
	}
	return buff.Bytes(), nil
}

func (step PlanStep) Name() string { return step.state.Name }

// Run runs the step's function if the step is not marked as complete, recording
// any error message to Err. If the step does not return an error, it is marked
// as complete and any previous error message is cleared.
func (step *PlanStep) Run(ctx context.Context) error {
	if step.run == nil {
		return nil
	}
	if step.state.Completed {
		return nil
	}
	size, err := step.run(ctx)
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
func (step *PlanStep) Revert(ctx context.Context) error {
	if step.revert == nil {
		return nil
	}
	if !step.state.Completed {
		return nil
	}
	err := step.revert(ctx)
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
// run action.
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
func updateDeclarationSteps(fsys ocflfs.FS, dir string, newSpec, oldSpec Spec) []PlanStep {
	steps := []PlanStep{}
	if newSpec == oldSpec {
		return steps
	}
	newDecl := Namaste{Type: NamasteTypeObject, Version: newSpec}
	newDeclName := path.Join(dir, newDecl.Name())
	steps = append(steps, PlanStep{
		state: planStepState{Name: "write " + newDeclName},
		run: func(ctx context.Context) (int64, error) {
			return 0, WriteDeclaration(ctx, fsys, dir, newDecl)
		},
		revert: func(ctx context.Context) error {
			err := ocflfs.Remove(ctx, fsys, newDeclName)
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		},
	})
	if !oldSpec.Empty() {
		oldDecl := Namaste{Type: NamasteTypeObject, Version: oldSpec}
		oldDeclName := path.Join(dir, oldDecl.Name())
		steps = append(steps, PlanStep{
			state: planStepState{Name: "remove " + oldDeclName},
			run: func(ctx context.Context) (int64, error) {
				err := ocflfs.Remove(ctx, fsys, oldDeclName)
				if errors.Is(err, fs.ErrNotExist) {
					err = nil
				}
				return 0, err
			},
			revert: func(ctx context.Context) error {
				return WriteDeclaration(ctx, fsys, dir, oldDecl)
			},
		})
	}
	return steps
}

// steps for copying files into the object's version directory
func updateVersionContentsSteps(objFS ocflfs.FS, objDir string, newContent PathMap, src ContentSource) []PlanStep {
	var steps []PlanStep
	for dstName, dig := range newContent.SortedPaths() {
		dstPath := path.Join(objDir, dstName)
		steps = append(steps, PlanStep{
			state: planStepState{
				Name:          "copy " + dstPath,
				ContentDigest: dig,
				ContentPath:   dstName,
				Async:         true,
			},
			run: func(ctx context.Context) (int64, error) {
				srcFS, srcPath := src.GetContent(dig)
				if srcFS == nil {
					return 0, fmt.Errorf("content source doesn't provide %q", dig)
				}
				return ocflfs.Copy(ctx, objFS, dstPath, srcFS, srcPath)
			},
			revert: func(ctx context.Context) error {
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
func updateInventoriesSteps(
	objFS ocflfs.FS, objDir string, newHead VNum,
	newInvBytes []byte, oldInvBytes []byte,
	newInvDigest string, oldInvDigest string,
	newAlg string, oldAlg string,
) []PlanStep {
	var steps []PlanStep
	rootInv := path.Join(objDir, inventoryBase)
	rootInvSidecar := rootInv + "." + newAlg

	verDir := path.Join(objDir, newHead.String())
	verDirInv := path.Join(verDir, inventoryBase)
	verDirInvSidecar := verDirInv + "." + newAlg
	var oldHead VNum
	if newHead.num > 1 {
		oldHead = VNum{num: newHead.num - 1, padding: newHead.padding}
	}
	// write version directory inventory.json
	steps = append(steps, PlanStep{
		state: planStepState{Name: "write " + verDirInv},
		run: func(ctx context.Context) (int64, error) {
			return ocflfs.Write(ctx, objFS, verDirInv, bytes.NewReader(newInvBytes))
		},
		revert: func(ctx context.Context) error {
			err := ocflfs.Remove(ctx, objFS, verDirInv)
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		},
	})
	// write version directory inventory sidecar
	steps = append(steps, PlanStep{
		state: planStepState{Name: "write " + verDirInvSidecar},
		run: func(ctx context.Context) (int64, error) {
			return 0, writeInventorySidecar(ctx, objFS, verDir, newInvDigest, newAlg)
		},
		revert: func(ctx context.Context) error {
			err := ocflfs.Remove(ctx, objFS, verDirInvSidecar)
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		},
	})
	// write root inventory.json
	steps = append(steps, PlanStep{
		state: planStepState{Name: "write " + rootInv},
		run: func(ctx context.Context) (int64, error) {
			return ocflfs.Write(ctx, objFS, rootInv, bytes.NewReader(newInvBytes))
		},
		revert: func(ctx context.Context) error {
			if newHead.num == 1 {
				err := ocflfs.Remove(ctx, objFS, rootInv)
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			// restore previous version directory: first, try copying from previous
			// version directory. If that doesn't work write oldInvBytes
			oldVerInv := path.Join(objDir, oldHead.String(), inventoryBase)
			_, err := ocflfs.Copy(ctx, objFS, rootInv, objFS, oldVerInv)
			if errors.Is(err, fs.ErrNotExist) && len(oldInvBytes) > 0 {
				// last version inventory didn't exist
				_, err = ocflfs.Write(ctx, objFS, rootInv, bytes.NewReader(oldInvBytes))
			}
			return err
		},
	})
	// write root inventory sidecar
	steps = append(steps, PlanStep{
		state: planStepState{Name: "set " + rootInvSidecar},
		run: func(ctx context.Context) (int64, error) {
			err := writeInventorySidecar(ctx, objFS, objDir, newInvDigest, newAlg)
			if err != nil {
				return 0, err
			}
			if oldAlg == "" || oldAlg == newAlg {
				return 0, nil
			}
			// previous sidecar used a different algorithm needs to be removed
			oldInvSidecar := rootInv + "." + oldAlg
			err = ocflfs.Remove(ctx, objFS, oldInvSidecar)
			if errors.Is(err, fs.ErrNotExist) {
				err = nil
			}
			return 0, err
		},
		revert: func(ctx context.Context) error {
			if newHead.num == 1 {
				err := ocflfs.Remove(ctx, objFS, rootInvSidecar)
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			// replace the new inventory sidecar with the old one. These
			// would be separate files -- we don't have to worry about that
			// because algorithms changes aren't supported
			if err := writeInventorySidecar(ctx, objFS, objDir, oldInvDigest, oldAlg); err != nil {
				return err
			}
			if oldAlg == "" || oldAlg == newAlg {
				return nil
			}
			// new sidecar uses a different algorithm and needs to be removed
			err := ocflfs.Remove(ctx, objFS, rootInvSidecar)
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		},
	})
	return steps
}

// run steps, forward or backward
func runSteps(ctx context.Context, steps iter.Seq[*PlanStep], gos int, logger *slog.Logger, backward bool) error {
	if gos < 1 {
		gos = runtime.NumCPU()
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	group := &errgroup.Group{}
	group.SetLimit(gos)
	for step := range steps {
		if step.state.Async {
			group.Go(func() error {
				var err error
				switch {
				case backward:
					logger.Info("reverting", "step", step.state.Name)
					err = step.Revert(ctx)
					if err != nil {
						logger.Error(err.Error())
					}
				default:
					logger.Info(step.state.Name)
					err = step.Run(ctx)
					if err != nil {
						logger.Error(err.Error())
					}
				}
				return err
			})
			continue
		}
		// wait for previous async steps to complete
		if err := group.Wait(); err != nil {
			return err
		}
		switch {
		case backward:
			logger.Info("reverting", "step", step.state.Name)
			if err := step.Revert(ctx); err != nil {
				logger.Error(err.Error())
				return err
			}
		default:
			logger.Info(step.state.Name)
			if err := step.Run(ctx); err != nil {
				logger.Error(err.Error())
				return err
			}
		}
	}
	return group.Wait()
}

type updatePlanState struct {
	NewInventoryBytes []byte
	OldInventoryBytes []byte
	Steps             []PlanStep
}

type planStepState struct {
	Name          string
	Err           string
	RevertErr     string
	Completed     bool
	Async         bool
	Size          int64
	ContentDigest string
	ContentPath   string
}
