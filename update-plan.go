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
	NewInventoryBytes []byte
	OldInventoryBytes []byte
	Steps             PlanSteps

	goLimit int
	logger  *slog.Logger
}

// NewUpdatePlan builds a new *UpdatePlan for updating obj's inventory and
// content.
func NewUpdatePlan(obj *Object, newInv *Inventory, src ContentSource) (*UpdatePlan, error) {
	if _, isWriteFS := obj.fs.(ocflfs.WriteFS); !isWriteFS {
		return nil, errors.New("object's backing file system doesn't support write operations")
	}
	oldInv := obj.rootInventory
	newInvBytes, invDigest, err := newInv.marshal()
	if err != nil {
		return nil, fmt.Errorf("building new inventory: %w", err)
	}
	u := &UpdatePlan{
		OldInventoryBytes: obj.rootInventoryBytes,
		NewInventoryBytes: newInvBytes,
	}
	InvDigest := &StoredInventory{Inventory: *newInv, digest: invDigest}
	steps, err := u.buildStepFunctions(obj.fs, obj.path, InvDigest, oldInv, src)
	if err != nil {
		return nil, fmt.Errorf("planning update for %q: %w", obj.ID(), err)
	}
	u.Steps = steps
	return u, nil
}

// RecoverUpdatePlan reconstitutes an *UpdatePlan from a previous
// UpdatePlan's byte representation.
func RecoverUpdatePlan(enc []byte, objFS ocflfs.FS, objDir string, src ContentSource) (*UpdatePlan, error) {
	var u UpdatePlan
	err := gob.NewDecoder(bytes.NewReader(enc)).Decode(&u)
	if err != nil {
		return nil, err
	}
	newInv, err := newStoredInventory(u.NewInventoryBytes)
	if err != nil {
		return nil, err
	}
	var oldInv *StoredInventory
	if len(u.OldInventoryBytes) > 0 {
		oldInv, err = newStoredInventory(u.OldInventoryBytes)
		if err != nil {
			return nil, err
		}
	}
	// the unmarshalled newSteps have Done and Err state, but their run functions
	// nil: rebuild the newSteps to run and import the previous run state.
	newSteps, err := u.buildStepFunctions(objFS, objDir, newInv, oldInv, src)
	if err != nil {
		return nil, err
	}
	if !newSteps.Eq(u.Steps) {
		return nil, errors.New("previous update log doesn't reflect current update plan")
	}
	for i := range newSteps {
		newSteps[i].Complete = u.Steps[i].Complete
		newSteps[i].Err = u.Steps[i].Err
	}
	u.Steps = newSteps
	return &u, nil
}

// Apply runs u's incomplete steps. It stops at the first error and returns the
// error. Consecutive steps with Async == true are run concurrently. Use
// SetGoLimit to set number of goroutines used for handling concurrent steps.
func (u *UpdatePlan) Apply(ctx context.Context) error {
	return runSteps(ctx, u.IncompleteSteps(), u.goLimit, u.logger, false)
}

// CompletedSteps is an iterator over completed steps in reverse order (most
// recently completed first).
func (u UpdatePlan) CompletedSteps() iter.Seq[*UpdateStep] {
	return func(yield func(*UpdateStep) bool) {
		for i := range slices.Backward(u.Steps) {
			step := &u.Steps[i]
			if !step.Complete {
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
	for _, step := range u.Steps {
		if step.Err != "" {
			errs = append(errs, errors.New(step.Err))
		}
	}
	return errors.Join(errs...)
}

// Marshal returns u's representation as a byte slice, which may be used with
// RecoverUpdatePlan to restore the original value.
func (u UpdatePlan) Marshal() ([]byte, error) {
	buff := &bytes.Buffer{}
	if err := gob.NewEncoder(buff).Encode(u); err != nil {
		return nil, err
	}
	return buff.Bytes(), nil
}

// IncompleteSteps is an iterator over incomplete steps in u. Incomplete steps
// may have errors.
func (u *UpdatePlan) IncompleteSteps() iter.Seq[*UpdateStep] {
	return func(yield func(*UpdateStep) bool) {
		for i := range u.Steps {
			step := &u.Steps[i]
			if step.Complete {
				continue
			}
			if !yield(step) {
				break
			}
		}
	}
}

// SetGoLimti sets the number of goroutines used for processing Steps with Async
// == true. The default value is runtime.NumCPU()
func (u *UpdatePlan) SetGoLimit(gos int) { u.goLimit = gos }

// SetLogger sets a logger that will be used when running stesp in u.
func (u *UpdatePlan) SetLogger(logger *slog.Logger) { u.logger = logger }

// Revert calls the 'Revert' function on all Completed steps in u's update plan
func (u *UpdatePlan) Revert(ctx context.Context) error {
	return runSteps(ctx, u.CompletedSteps(), u.goLimit, u.logger, true)
}

func (u *UpdatePlan) buildStepFunctions(objFS ocflfs.FS, objDir string, newInv, oldInv *StoredInventory, src ContentSource) (PlanSteps, error) {
	newFiles := newInv.versionContent(newInv.Head)
	newInvDigest := newInv.digest
	newHead := newInv.Head
	newSpec := newInv.Type.Spec
	newAlg := newInv.DigestAlgorithm
	var oldHead VNum
	var oldSpec Spec
	var oldInvDigest, oldAlg string
	if oldInv != nil {
		oldInvDigest = oldInv.digest
		oldHead = oldInv.Head
		oldSpec = oldInv.Type.Spec
		oldAlg = oldInv.DigestAlgorithm
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
	var plan []UpdateStep
	// initial step (final undo step) to remove the entire object root
	// when undoing a v1 update. This is needed to make sure we get rid
	// of all empty directories.
	plan = append(plan, UpdateStep{
		Name: "object root " + objDir,
		run:  func(ctx context.Context) error { return nil },
		revert: func(ctx context.Context) error {
			// remove everything if we're undoing v1
			if newHead.num == 1 {
				return ocflfs.RemoveAll(ctx, objFS, objDir)
			}
			return nil
		},
	})
	// steps for object declaration files
	plan = append(plan, updateDeclarationSteps(objFS, objDir, newSpec, oldSpec)...)
	// a step to remove entire version directory during rollback. This is needed
	// to make sure we get rid of empty directories.
	verDir := path.Join(objDir, newHead.String())
	plan = append(plan, UpdateStep{
		Name: "version directory " + verDir,
		run:  func(ctx context.Context) error { return nil },
		revert: func(ctx context.Context) error {
			// remove everything in the version directory
			return ocflfs.RemoveAll(ctx, objFS, verDir)
		},
	})
	// steps to copy contents into the version directory
	plan = append(plan, updateVersionContentsSteps(objFS, objDir, newFiles, src)...)
	// steps to update inventories and sidecars in version directory and root
	plan = append(plan, updateInventoriesSteps(
		objFS, objDir, newHead,
		u.NewInventoryBytes, u.OldInventoryBytes,
		newInvDigest, oldInvDigest,
		newAlg, oldAlg,
	)...)
	return plan, nil
}

// PlanSteps is a series of named steps for performating an object update and
// rolling it back if necessary.
type PlanSteps []UpdateStep

func (s PlanSteps) Eq(other PlanSteps) bool {
	if len(s) != len(other) {
		return false
	}
	for i := range s {
		if s[i].Name != other[i].Name {
			return false
		}
	}
	return true
}

// UndoStep is a single step in an UpdatePlan.
type UpdateStep struct {
	// Descrptive name for the steps actions
	Name string `json:"name"`
	// Err has any error message from running the step
	Err string `json:"error,omitempty"`
	// RevertErr has any error message from reverting the step
	RevertErr string `json:"error_revert,omitempty"`
	// Complete is set to true if the Step ran without any error.
	// It is set to false if the step is reverted without any error.
	Complete bool `json:"complete,omitempty"`
	// Async indicates that the step can be run concurrently with adjacent
	// Async steps.
	Async bool `json:"async,omitempty"`

	// run performs the step's actions
	run func(ctx context.Context) error
	// revert reverts the run step
	revert func(ctx context.Context) error
}

// Run runs the step's function if the step is not marked as complete, recording
// any error message to Err. If the step returns no error, it is marked as
// complete and any previous Err message is cleared.
func (step *UpdateStep) Run(ctx context.Context) error {
	if step.run == nil {
		return nil
	}
	if step.Complete {
		return nil
	}
	if err := step.run(ctx); err != nil {
		msg := err.Error()
		if msg == "" {
			msg = "unspecified error"
		}
		step.Err = msg
		return err
	}
	step.Complete = true
	step.Err = ""
	return nil
}

// Revert calls step's undo function if the step is marked as complete. If the
// undo function returns an error, the error message is saved as RevertErr. If
// Revert does not result in an error the step is marked as incomplete and
// UndoErr is cleared.
func (step *UpdateStep) Revert(ctx context.Context) error {
	if step.revert == nil {
		return nil
	}
	if !step.Complete {
		return nil
	}
	err := step.revert(ctx)
	if err != nil {
		msg := err.Error()
		if msg == "" {
			msg = "unspecified error"
		}
		step.RevertErr = msg
		return err
	}
	step.Complete = false
	step.RevertErr = ""
	return nil
}

// steps for setting/updating an ocfl object declaration
func updateDeclarationSteps(fsys ocflfs.FS, dir string, newSpec, oldSpec Spec) []UpdateStep {
	steps := []UpdateStep{}

	if newSpec == oldSpec {
		return steps
	}
	newDecl := Namaste{Type: NamasteTypeObject, Version: newSpec}
	newDeclName := path.Join(dir, newDecl.Name())
	steps = append(steps, UpdateStep{
		Name: "write " + newDeclName,
		run: func(ctx context.Context) error {
			return WriteDeclaration(ctx, fsys, dir, newDecl)
		},
		revert: func(ctx context.Context) error {
			return ocflfs.Remove(ctx, fsys, newDeclName)
		},
	})
	if !oldSpec.Empty() {
		oldDecl := Namaste{Type: NamasteTypeObject, Version: oldSpec}
		oldDeclName := path.Join(dir, oldDecl.Name())
		steps = append(steps, UpdateStep{
			Name: "remove " + oldDeclName,
			run: func(ctx context.Context) error {
				return ocflfs.Remove(ctx, fsys, oldDeclName)
			},
			revert: func(ctx context.Context) error {
				return WriteDeclaration(ctx, fsys, dir, oldDecl)
			},
		})
	}
	return steps
}

// steps for copying files into the object's version directory
func updateVersionContentsSteps(objFS ocflfs.FS, objDir string, newContent PathMap, src ContentSource) []UpdateStep {
	var steps []UpdateStep
	for dstName, dig := range newContent.SortedPaths() {
		dstPath := path.Join(objDir, dstName)
		steps = append(steps, UpdateStep{
			Name:  "copy " + dstPath,
			Async: true,
			run: func(ctx context.Context) error {
				srcFS, srcPath := src.GetContent(dig)
				if srcFS == nil {
					return fmt.Errorf("content source doesn't provide %q", dig)
				}
				return ocflfs.Copy(ctx, objFS, dstPath, srcFS, srcPath)
			},
			revert: func(ctx context.Context) error {
				return ocflfs.Remove(ctx, objFS, dstPath)
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
) []UpdateStep {
	var steps []UpdateStep
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
	steps = append(steps, UpdateStep{
		Name: "write " + verDirInv,
		run: func(ctx context.Context) error {
			_, err := ocflfs.Write(ctx, objFS, verDirInv, bytes.NewReader(newInvBytes))
			return err
		},
		revert: func(ctx context.Context) error {
			return ocflfs.Remove(ctx, objFS, verDirInv)
		},
	})
	// write version directory inventory sidecar
	steps = append(steps, UpdateStep{
		Name: "write " + verDirInvSidecar,
		run: func(ctx context.Context) error {
			return writeInventorySidecar(ctx, objFS, verDir, newInvDigest, newAlg)
		},
		revert: func(ctx context.Context) error {
			return ocflfs.Remove(ctx, objFS, verDirInvSidecar)
		},
	})
	// write root inventory.json
	steps = append(steps, UpdateStep{
		Name: "write " + rootInv,
		run: func(ctx context.Context) error {
			_, err := ocflfs.Write(ctx, objFS, rootInv, bytes.NewReader(newInvBytes))
			return err
		},
		revert: func(ctx context.Context) error {
			if newHead.num == 1 {
				return ocflfs.Remove(ctx, objFS, rootInv)
			}
			// restore previous version directory: first, try copying from previous
			// version directory. If that doesn't work write oldInvBytes
			oldVerInv := path.Join(objDir, oldHead.String(), inventoryBase)
			err := ocflfs.Copy(ctx, objFS, rootInv, objFS, oldVerInv)
			if errors.Is(err, fs.ErrNotExist) && len(oldInvBytes) > 0 {
				// last version inventory didn't exist
				_, err = ocflfs.Write(ctx, objFS, rootInv, bytes.NewReader(oldInvBytes))
			}
			return err
		},
	})
	// write root inventory sidecar
	steps = append(steps, UpdateStep{
		Name: "set " + rootInvSidecar,
		run: func(ctx context.Context) error {
			err := writeInventorySidecar(ctx, objFS, objDir, newInvDigest, newAlg)
			if err != nil {
				return err
			}
			if oldAlg == "" || oldAlg == newAlg {
				return nil
			}
			// previous sidecar used a different algorithm needs to be removed
			oldInvSidecar := rootInv + "." + oldAlg
			return ocflfs.Remove(ctx, objFS, oldInvSidecar)
		},
		revert: func(ctx context.Context) error {
			if newHead.num == 1 {
				return ocflfs.Remove(ctx, objFS, rootInvSidecar)
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
			return ocflfs.Remove(ctx, objFS, rootInvSidecar)
		},
	})
	return steps
}

// run steps, forward or backward
func runSteps(ctx context.Context, steps iter.Seq[*UpdateStep], gos int, logger *slog.Logger, backward bool) error {
	if gos < 1 {
		gos = runtime.NumCPU()
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	group := &errgroup.Group{}
	group.SetLimit(gos)
	for step := range steps {
		if step.Async {
			group.Go(func() error {
				var err error
				switch {
				case backward:
					logger.Info("reverting", "step", step.Name)
					err = step.Revert(ctx)
					if err != nil {
						logger.Error(err.Error())
					}
				default:
					logger.Info(step.Name)
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
			logger.Info("reverting", "step", step.Name)
			if err := step.Revert(ctx); err != nil {
				logger.Error(err.Error())
				return err
			}
		default:
			logger.Info(step.Name)
			if err := step.Run(ctx); err != nil {
				logger.Error(err.Error())
				return err
			}
		}
	}
	return group.Wait()
}
