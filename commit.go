package ocfl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"time"

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// Commit represents an update to object.
type Commit struct {
	ID      string // required for new objects in storage roots without a layout.
	Stage   *Stage // required
	Message string // required
	User    User   // required

	// advanced options
	Created         time.Time // time.Now is used, if not set
	Spec            Spec      // OCFL specification version for the new object version
	NewHEAD         int       // enforces new object version number
	AllowUnchanged  bool
	ContentPathFunc func(oldPaths []string) (newPaths []string)

	Logger *slog.Logger
}

type CommitPlan struct {
	NewInventory *Inventory   `json:"new_inventory"`
	Steps        []CommitStep `json:"steps"`
}

func (s *CommitPlan) Apply(ctx context.Context) error {
	for _, step := range s.Steps {
		if err := step.Run(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *CommitPlan) append(step CommitStep) {
	s.Steps = append(s.Steps, step)
}

type CommitStep struct {
	Name string `json:"name"`
	//Async          []CommitStep `json:"async,omitempty"`
	Err  string `json:"error,omitempty"`
	Done bool   `json:"done,omitempty"`
	//CompensateErr  string       `json:"compensate_error"`
	//CompensateDone bool         `json:"compensate_done"`

	run        func(ctx context.Context) error
	compensate func(ctx context.Context) error
}

func (step *CommitStep) Run(ctx context.Context) error {
	if step.run != nil {
		if err := step.run(ctx); err != nil {
			step.Err = err.Error()
			return err
		}
	}
	step.Done = true
	return nil
}

// Commit error wraps an error from a commit.
type CommitError struct {
	Err error // The wrapped error

	// Dirty indicates the object may be incomplete or invalid as a result of
	// the error.
	Dirty bool
}

func (c CommitError) Error() string {
	return c.Err.Error()
}

func (c CommitError) Unwrap() error {
	return c.Err
}

type CommitSaga struct {
	ObjectFS           ocflfs.FS
	ObjectDir          string
	Source             ContentSource
	NewInventory       *Inventory
	PrevInventory      *Inventory
	PrevInventoryBytes []byte

	steps []CommitSagaStep
}

func (s CommitSaga) Run(ctx context.Context, fsys ocflfs.FS, dir string, src ContentSource) error {
	return nil
}

func commitStepsObjectDeclaration(fsys ocflfs.FS, dir string, newSpec, oldSpec Spec) []CommitSagaStep {
	var steps []CommitSagaStep
	if newSpec == oldSpec {
		return steps
	}
	newDecl := Namaste{Type: NamasteTypeObject, Version: newSpec}
	newDeclName := path.Join(dir, newDecl.Name())
	steps = append(steps, CommitSagaStep{
		Name: "write " + newDeclName,
		run: func(ctx context.Context) error {
			return WriteDeclaration(ctx, fsys, dir, newDecl)
		},
		compensate: func(ctx context.Context) error {
			return ocflfs.Remove(ctx, fsys, newDeclName)
		},
	})
	if !oldSpec.Empty() {
		oldDecl := Namaste{Type: NamasteTypeObject, Version: oldSpec}
		oldDeclName := path.Join(dir, oldDecl.Name())
		steps = append(steps, CommitSagaStep{
			Name: "remove" + oldDeclName,
			run: func(ctx context.Context) error {
				return ocflfs.Remove(ctx, fsys, oldDeclName)
			},
			compensate: func(ctx context.Context) error {
				return WriteDeclaration(ctx, fsys, dir, oldDecl)
			},
		})
	}
	return steps
}

func commitStepsCopyContents(objFS ocflfs.FS, objDir string, newInv *Inventory, src ContentSource) []CommitSagaStep {
	var steps []CommitSagaStep
	for dstName, dig := range newInv.versionContent(newInv.Head).SortedPaths() {
		dstPath := path.Join(objDir, dstName)
		steps = append(steps, CommitSagaStep{
			Name: "copy" + dstPath,
			run: func(ctx context.Context) error {
				srcFS, srcPath := src.GetContent(dig)
				if srcFS == nil {
					return fmt.Errorf("content source doesn't provide %q", dig)
				}
				return ocflfs.Copy(ctx, objFS, dstPath, srcFS, srcPath)
			},
			compensate: func(ctx context.Context) error {
				return ocflfs.Remove(ctx, objFS, dstPath)
			},
		})
	}
	return steps
}

func updateInventorySteps(objFS ocflfs.FS, objDir string, newInv, lastInv *Inventory) []CommitSagaStep {
	var steps []CommitSagaStep
	rootInv := path.Join(objDir, inventoryBase)
	rootInvSidecar := rootInv + "." + newInv.DigestAlgorithm
	verDir := path.Join(objDir, newInv.Head.String())
	verDirInv := path.Join(verDir, inventoryBase)
	verDirInvSidecar := verDirInv + "." + newInv.DigestAlgorithm
	// write version directory inventory.json
	steps = append(steps, CommitSagaStep{
		Name: "write " + verDirInv,
		run: func(ctx context.Context) error {
			_, err := ocflfs.Write(ctx, objFS, verDirInv, bytes.NewReader(newInv.raw))
			return err
		},
		compensate: func(ctx context.Context) error {
			return ocflfs.Remove(ctx, objFS, verDirInv)
		},
	})
	// write version directory inventory sidecar
	steps = append(steps, CommitSagaStep{
		Name: "write " + verDirInvSidecar,
		run: func(ctx context.Context) error {
			return writeInventorySidecar(ctx, objFS, verDir, newInv.rawDigest, newInv.DigestAlgorithm)
		},
		compensate: func(ctx context.Context) error {
			return ocflfs.Remove(ctx, objFS, verDirInvSidecar)
		},
	})
	// write root inventory.json
	steps = append(steps, CommitSagaStep{
		Name: "write " + rootInv,
		run: func(ctx context.Context) error {
			_, err := ocflfs.Write(ctx, objFS, rootInv, bytes.NewReader(newInv.raw))
			return err
		},
		compensate: func(ctx context.Context) error {
			if newInv.Head.num == 1 {
				return ocflfs.Remove(ctx, objFS, rootInv)
			}
			// restore previous version directory: first, try copying from previous
			// version directory. If that doesn't work write lastInv
			lastVer := V(newInv.Head.num-1, newInv.Head.padding).String()
			lastVerInv := path.Join(objDir, lastVer, inventoryBase)
			err := ocflfs.Copy(ctx, objFS, rootInv, objFS, lastVerInv)
			if errors.Is(err, fs.ErrNotExist) && lastInv != nil && len(lastInv.raw) > 0 {
				// last version inventory didn't exist
				_, err = ocflfs.Write(ctx, objFS, rootInv, bytes.NewReader(lastInv.raw))
			}
			return err
		},
	})
	// write root inventory sidecar
	steps = append(steps, CommitSagaStep{
		Name: "set " + rootInvSidecar,
		run: func(ctx context.Context) error {
			err := writeInventorySidecar(ctx, objFS, objDir, newInv.rawDigest, newInv.DigestAlgorithm)
			if err != nil {
				return err
			}
			if lastInv == nil || lastInv.DigestAlgorithm == newInv.DigestAlgorithm {
				return nil
			}
			// previous sidecar used a different algorithm and it needs to be removed
			lastInvSidecar := path.Join(objDir, inventoryBase+"."+lastInv.DigestAlgorithm)
			return ocflfs.Remove(ctx, objFS, lastInvSidecar)
		},
		compensate: func(ctx context.Context) error {
			// writing the root inventory sidecar is the final set for an object update --
			// there should be no need to compensate.
			return nil
		},
	})
	return steps
}

type CommitSagaStep struct {
	Name       string `json:"name"`
	run        func(ctx context.Context) error
	compensate func(ctx context.Context) error
}

type CommitSagaLogEntry struct {
	Name string `json:"name"`
	//Async          []CommitStep `json:"async,omitempty"`
	Err  string `json:"error,omitempty"`
	Done bool   `json:"done,omitempty"`
	//CompensateErr  string       `json:"compensate_error"`
	//CompensateDone bool         `json:"compensate_done"`
}
