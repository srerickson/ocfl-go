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
	"golang.org/x/sync/errgroup"
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

type CommitSteps []CommitStep

func NewCommitSteps(fsys ocflfs.FS, dir string, newInv *Inventory, src ContentSource) (CommitSteps, error) {
	var lastSpec, newSpec Spec
	newSpec = newInv.Type.Spec
	if newInv.prev != nil {
		lastSpec = newInv.prev.Type.Spec
	}
	steps, err := commitObjectDeclaration(fsys, dir, newSpec, lastSpec)
	if err != nil {
		return nil, err
	}
	nextSteps, err := commitContents(fsys, dir, newInv, src)
	if err != nil {
		return nil, err
	}
	steps = append(steps, nextSteps...)
	nextSteps, err = commitInventories(fsys, dir, newInv)
	if err != nil {
		return nil, err
	}
	steps = append(steps, nextSteps...)
	return steps, nil
}

func (s CommitSteps) ApplyLog(prev CommitSteps) error {
	err := errors.New("previous commit log doesn't match current commit plan")
	if len(s) != len(prev) {
		return err
	}
	for i := range s {
		if s[i].Name != prev[i].Name {
			return err
		}
	}
	for i := range s {
		s[i].Done = prev[i].Done
		s[i].Err = prev[i].Err
	}
	return nil
}

// RunAsync calls Run for every step in s that is not Done. Consecutive steps with Async == true are
// run concurrently using gos goroutines.
func (s CommitSteps) RunAsync(ctx context.Context, gos int) error {
	group := &errgroup.Group{}
	group.SetLimit(gos)
	for i := range s {
		step := &s[i]
		if step.Done {
			continue
		}
		if step.Async {
			group.Go(func() error { return step.Run(ctx) })
			continue
		}
		// wait for previous async steps to complete
		if err := group.Wait(); err != nil {
			return err
		}
		if err := step.Run(ctx); err != nil {
			return err
		}
	}
	return group.Wait()
}

type CommitStep struct {
	Name  string `json:"name"`
	Err   string `json:"error,omitempty"`
	Done  bool   `json:"done,omitempty"`
	Async bool   `json:"async,omitempty"`
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

func commitObjectDeclaration(fsys ocflfs.FS, dir string, newSpec, oldSpec Spec) ([]CommitStep, error) {
	var steps []CommitStep
	if newSpec == oldSpec {
		return steps, nil
	}
	if newSpec.Cmp(oldSpec) < 0 {
		err := fmt.Errorf("new version's OCFL spec (%q) cannot be lower than the previous version's (%q)", newSpec, oldSpec)
		return nil, err
	}
	newDecl := Namaste{Type: NamasteTypeObject, Version: newSpec}
	newDeclName := path.Join(dir, newDecl.Name())
	steps = append(steps, CommitStep{
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
		steps = append(steps, CommitStep{
			Name: "remove" + oldDeclName,
			run: func(ctx context.Context) error {
				return ocflfs.Remove(ctx, fsys, oldDeclName)
			},
			compensate: func(ctx context.Context) error {
				return WriteDeclaration(ctx, fsys, dir, oldDecl)
			},
		})
	}
	return steps, nil
}

func commitContents(objFS ocflfs.FS, objDir string, newInv *Inventory, src ContentSource) ([]CommitStep, error) {
	newContent := newInv.versionContent(newInv.Head).SortedPaths()
	for _, dig := range newContent {
		if fsys, _ := src.GetContent(dig); fsys == nil {
			return nil, fmt.Errorf("content source doesn't provide %q", dig)
		}
	}
	var steps []CommitStep
	for dstName, dig := range newContent {
		dstPath := path.Join(objDir, dstName)
		steps = append(steps, CommitStep{
			Name:  "copy" + dstPath,
			Async: true,
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
	return steps, nil
}

// steps for updating object's version and root inventories. the newInv should
// have been constructed with inventory builder.
func commitInventories(objFS ocflfs.FS, objDir string, newInv *Inventory) ([]CommitStep, error) {
	var steps []CommitStep
	lastInv := newInv.prev
	if newInv.Head.num > 1 {
		if lastInv == nil {
			return nil, errors.New("inventory is missing its previous inventory reference")
		}
		if newInv.Head.num != lastInv.Head.num+1 {
			return nil, errors.New("new inventory includes more than one new version")
		}
	}
	if err := newInv.marshal(); err != nil {
		return nil, err
	}
	rootInv := path.Join(objDir, inventoryBase)
	rootInvSidecar := rootInv + "." + newInv.DigestAlgorithm
	verDir := path.Join(objDir, newInv.Head.String())
	verDirInv := path.Join(verDir, inventoryBase)
	verDirInvSidecar := verDirInv + "." + newInv.DigestAlgorithm
	// write version directory inventory.json
	steps = append(steps, CommitStep{
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
	steps = append(steps, CommitStep{
		Name: "write " + verDirInvSidecar,
		run: func(ctx context.Context) error {
			return writeInventorySidecar(ctx, objFS, verDir, newInv.rawDigest, newInv.DigestAlgorithm)
		},
		compensate: func(ctx context.Context) error {
			return ocflfs.Remove(ctx, objFS, verDirInvSidecar)
		},
	})
	// write root inventory.json
	steps = append(steps, CommitStep{
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
	steps = append(steps, CommitStep{
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
	return steps, nil
}
