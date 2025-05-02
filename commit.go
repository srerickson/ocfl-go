package ocfl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"

	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/logging"
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

// commitPlan represents the set of actions need to complete an object update.
type commitPlan struct {
	FS            ocflfs.FS
	Path          string
	NewInventory  *Inventory
	PrevInventoy  *Inventory
	NewContent    DigestMap
	ContentSource ContentSource
}

func (p *commitPlan) Run(ctx context.Context, logger *slog.Logger) error {
	if logger == nil {
		logger = logging.DisabledLogger()
	}
	// file changes start here
	// 1. create or update NAMASTE object declaration
	newSpec := p.NewInventory.Type.Spec
	var oldSpec Spec
	if p.PrevInventoy != nil {
		oldSpec = p.PrevInventoy.Type.Spec
	}
	if oldSpec != newSpec {
		if !oldSpec.Empty() {
			oldDecl := Namaste{Type: NamasteTypeObject, Version: oldSpec}
			logger.DebugContext(ctx, "deleting previous OCFL object declaration", "name", oldDecl)
			if err := ocflfs.Remove(ctx, p.FS, path.Join(p.Path, oldDecl.Name())); err != nil {
				return &CommitError{Err: err, Dirty: true}
			}
		}
		newDecl := Namaste{Type: NamasteTypeObject, Version: newSpec}
		logger.DebugContext(ctx, "writing new OCFL object declaration", "name", newDecl)
		if err := WriteDeclaration(ctx, p.FS, p.Path, newDecl); err != nil {
			return &CommitError{Err: err, Dirty: true}
		}
	}
	// 2. tranfser files from stage to object
	if len(p.NewContent) > 0 {
		copyOpts := &copyContentOpts{
			Source:   p.ContentSource,
			DestFS:   p.FS,
			DestRoot: p.Path,
			Manifest: p.NewContent,
		}
		logger.DebugContext(ctx, "copying new object files", "count", len(p.NewContent))
		if err := copyContent(ctx, copyOpts); err != nil {
			err = fmt.Errorf("transferring new object contents: %w", err)
			return &CommitError{Err: err, Dirty: true}
		}
	}
	logger.DebugContext(ctx, "writing inventories for new object version")
	// 3. write inventory to both object root and version directory
	newVersionDir := path.Join(p.Path, p.NewInventory.Head.String())
	if err := writeInventory(ctx, p.FS, p.NewInventory, p.Path, newVersionDir); err != nil {
		err = fmt.Errorf("writing new inventories or inventory sidecars: %w", err)
		return &CommitError{Err: err, Dirty: true}
	}
	return nil
}

// newContentMap returns a DigestMap that is a subset of the inventory
// manifest for the digests and paths of new content
func newContentMap(inv *Inventory) (DigestMap, error) {
	pm := PathMap{}
	for pth, dig := range inv.Manifest.Paths() {
		// ignore manifest entries from previous versions
		if !strings.HasPrefix(pth, inv.Head.String()+"/") {
			continue
		}
		if _, exists := pm[pth]; exists {
			return nil, fmt.Errorf("path duplicate in manifest: %q", pth)
		}
		pm[pth] = dig
	}
	dm := pm.DigestMap()
	if err := dm.Valid(); err != nil {
		return nil, err
	}
	return dm, nil
}

type copyContentOpts struct {
	Source      ContentSource
	DestFS      ocflfs.FS
	DestRoot    string
	Manifest    DigestMap
	Concurrency int
}

// transfer dst/src names in files from srcFS to dstFS
func copyContent(ctx context.Context, c *copyContentOpts) error {
	if c.Source == nil {
		return errors.New("missing countent source")
	}
	conc := c.Concurrency
	if conc < 1 {
		conc = 1
	}
	grp, ctx := errgroup.WithContext(ctx)
	grp.SetLimit(conc)
	for dig, dstNames := range c.Manifest {
		srcFS, srcPath := c.Source.GetContent(dig)
		if srcFS == nil {
			return fmt.Errorf("content source doesn't provide %q", dig)
		}
		for _, dstName := range dstNames {
			srcPath := srcPath
			dstPath := path.Join(c.DestRoot, dstName)
			grp.Go(func() error {
				return ocflfs.Copy(ctx, c.DestFS, dstPath, srcFS, srcPath)
			})

		}
	}
	return grp.Wait()
}
