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

// Commit creates or updates the OCFL v1.x object objectID at objPath using the
// content from stage. If an object exists at objPath, the object will be
// updated; if objPath does not exist, a new object will be created. If objPath
// exists but it is not the root of a valid OCFL object, an error is returned.
// The digest algorithm for the object version is taken from the stage. For
// object updates, the stage's algorithm must match the existing object's digest
// algorithm. The error returned by commit is always a CommitError.
func commit(ctx context.Context, prev ocfl.SpecObject, commit *ocfl.Commit) (err error) {
	writeFS, ok := prev.FS().(ocfl.WriteFS)
	if !ok {
		return errors.New("object's backing file system doesn't support write operations")
	}
	logger := commit.Logger
	if logger == nil {
		logger = logging.DisabledLogger()
	}
	if commit.Stage == nil {
		return &ocfl.CommitError{Err: errors.New("commit's stage is missing")}
	}
	if commit.Stage.State == nil {
		commit.Stage.State = ocfl.DigestMap{}
	}
	newInv, err := NewInventory(commit, prev.Inventory())
	if err != nil {
		return &ocfl.CommitError{Err: err}
	}
	logger = logger.With("head", newInv.Head, "ocfl_spec", newInv.Type.Spec, "alg", newInv.DigestAlgorithm)
	// xfers is a subeset of the manifest with the new content to add
	xfers, err := newContentMap(newInv)
	if err != nil {
		return &ocfl.CommitError{Err: err}
	}
	// check that the stage includes all the new content
	for digest := range xfers {
		if !commit.Stage.HasContent(digest) {
			err := fmt.Errorf("stage's content source doesn't provide digest: %s", digest)
			return &ocfl.CommitError{Err: err}
		}
	}
	// file changes start here
	// 1. create or update NAMASTE object declaration
	var oldSpec ocfl.Spec
	if prev.Inventory() != nil {
		oldSpec = prev.Inventory().Spec()
	}
	newSpec := newInv.Type.Spec
	switch {
	case ocfl.ObjectExists(prev) && oldSpec != newSpec:
		oldDecl := ocfl.Namaste{Type: ocfl.NamasteTypeObject, Version: oldSpec}
		logger.DebugContext(ctx, "deleting previous OCFL object declaration", "name", oldDecl)
		if err = writeFS.Remove(ctx, path.Join(prev.Path(), oldDecl.Name())); err != nil {
			return &ocfl.CommitError{Err: err, Dirty: true}
		}
		fallthrough
	case !ocfl.ObjectExists(prev):
		newDecl := ocfl.Namaste{Type: ocfl.NamasteTypeObject, Version: newSpec}
		logger.DebugContext(ctx, "writing new OCFL object declaration", "name", newDecl)
		if err = ocfl.WriteDeclaration(ctx, writeFS, prev.Path(), newDecl); err != nil {
			return &ocfl.CommitError{Err: err, Dirty: true}
		}
	}
	// 2. tranfser files from stage to object
	if len(xfers) > 0 {
		copyOpts := &commitCopyOpts{
			Source:   commit.Stage,
			DestFS:   writeFS,
			DestRoot: prev.Path(),
			Manifest: xfers,
		}
		if err = commitCopy(ctx, copyOpts); err != nil {
			err = fmt.Errorf("transferring new object contents: %w", err)
			return &ocfl.CommitError{Err: err, Dirty: true}
		}
	}
	logger.DebugContext(ctx, "writing inventories for new object version")
	// 3. write inventory to both object root and version directory
	newVersionDir := path.Join(prev.Path(), newInv.Head.String())
	if err = WriteInventory(ctx, writeFS, newInv, prev.Path(), newVersionDir); err != nil {
		err = fmt.Errorf("writing new inventories or inventory sidecars: %w", err)
		return &ocfl.CommitError{Err: err, Dirty: true}
	}
	return nil
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
