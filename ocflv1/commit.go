package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/logging"
	"golang.org/x/sync/errgroup"
)

func doCommit(ctx context.Context, obj ocfl.ObjectReader, commit *ocfl.Commit) (*RawInventory, error) {
	writeFS, ok := obj.FS().(ocfl.WriteFS)
	if !ok {
		err := errors.New("object's backing file system doesn't support write operations")
		return nil, &ocfl.CommitError{Err: err}
	}
	newInv, err := buildInventory(obj.Inventory(), commit)
	if err != nil {
		err := fmt.Errorf("building new inventory: %w", err)
		return nil, &ocfl.CommitError{Err: err}
	}
	logger := commit.Logger
	if logger == nil {
		logger = logging.DisabledLogger()
	}
	logger = logger.With("path", obj.Path(), "id", newInv.ID, "head", newInv.Head, "ocfl_spec", newInv.Type.Spec, "alg", newInv.DigestAlgorithm)
	// xfers is a subeset of the manifest with the new content to add
	xfers, err := newContentMap(newInv)
	if err != nil {
		return nil, &ocfl.CommitError{Err: err}
	}
	// check that the stage includes all the new content
	for digest := range xfers {
		if !commit.Stage.HasContent(digest) {
			// FIXME short digest
			err := fmt.Errorf("no content for digest: %s", digest)
			return nil, &ocfl.CommitError{Err: err}
		}
	}
	// file changes start here
	// 1. create or update NAMASTE object declaration
	var oldSpec ocfl.Spec
	if obj.Inventory() != nil {
		oldSpec = obj.Inventory().Spec()
	}
	newSpec := newInv.Type.Spec
	switch {
	case ocfl.ObjectExists(obj) && oldSpec != newSpec:
		oldDecl := ocfl.Namaste{Type: ocfl.NamasteTypeObject, Version: oldSpec}
		logger.DebugContext(ctx, "deleting previous OCFL object declaration", "name", oldDecl)
		if err = writeFS.Remove(ctx, path.Join(obj.Path(), oldDecl.Name())); err != nil {
			return nil, &ocfl.CommitError{Err: err, Dirty: true}
		}
		fallthrough
	case !ocfl.ObjectExists(obj):
		newDecl := ocfl.Namaste{Type: ocfl.NamasteTypeObject, Version: newSpec}
		logger.DebugContext(ctx, "writing new OCFL object declaration", "name", newDecl)
		if err = ocfl.WriteDeclaration(ctx, writeFS, obj.Path(), newDecl); err != nil {
			return nil, &ocfl.CommitError{Err: err, Dirty: true}
		}
	}
	// 2. tranfser files from stage to object
	if len(xfers) > 0 {
		copyOpts := &commitCopyOpts{
			Source:   commit.Stage,
			DestFS:   writeFS,
			DestRoot: obj.Path(),
			Manifest: xfers,
		}
		logger.DebugContext(ctx, "copying new object files", "count", len(xfers))
		if err := commitCopy(ctx, copyOpts); err != nil {
			err = fmt.Errorf("transferring new object contents: %w", err)
			return nil, &ocfl.CommitError{Err: err, Dirty: true}
		}
	}
	logger.DebugContext(ctx, "writing inventories for new object version")
	// 3. write inventory to both object root and version directory
	newVersionDir := path.Join(obj.Path(), newInv.Head.String())
	if err := writeInventory(ctx, writeFS, newInv, obj.Path(), newVersionDir); err != nil {
		err = fmt.Errorf("writing new inventories or inventory sidecars: %w", err)
		return nil, &ocfl.CommitError{Err: err, Dirty: true}
	}
	return newInv, nil
}

// newContentMap returns a DigestMap that is a subset of the inventory
// manifest for the digests and paths of new content
func newContentMap(inv *RawInventory) (ocfl.DigestMap, error) {
	pm := ocfl.PathMap{}
	var err error
	inv.Manifest.EachPath(func(pth, dig string) bool {
		// ignore manifest entries from previous versions
		if !strings.HasPrefix(pth, inv.Head.String()+"/") {
			return true
		}
		if _, exists := pm[pth]; exists {
			err = fmt.Errorf("path duplicate in manifest: %q", pth)
			return false
		}
		pm[pth] = dig
		return true
	})
	if err != nil {
		return nil, err
	}
	return pm.DigestMapValid()
}

type commitCopyOpts struct {
	Source      ocfl.ContentSource
	DestFS      ocfl.WriteFS
	DestRoot    string
	Manifest    ocfl.DigestMap
	Concurrency int
}

// transfer dst/src names in files from srcFS to dstFS
func commitCopy(ctx context.Context, c *commitCopyOpts) error {
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
				return ocfl.Copy(ctx, c.DestFS, dstPath, srcFS, srcPath)
			})

		}
	}
	return grp.Wait()
}
