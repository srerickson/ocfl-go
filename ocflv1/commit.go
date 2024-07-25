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
func commit(ctx context.Context, obj ocfl.SpecObject, commit *ocfl.Commit, newSpec ocfl.Spec) (err error) {
	writeFS, ok := obj.FS().(ocfl.WriteFS)
	if !ok {
		return errors.New("object's backing file system doesn't support write operations")
	}
	logger := commit.Logger
	if logger == nil {
		logger = logging.DisabledLogger()
	}
	if commit.Stage.State == nil {
		commit.Stage.State = ocfl.DigestMap{}
	}
	newInv, err := NewInventory(commit, obj.Inventory())
	if err != nil {
		return err
	}
	newInv.Type = newSpec.AsInvType()

	// switch {
	// case obj.Exists():
	// 	currentInventory := obj.Inventory()
	// 	currentID := currentInventory.ID()
	// 	currentAlg := currentInventory.DigestAlgorithm()
	// 	logger = logger.With("object_path", obj.Path(), "object_id", currentInventory.ID())
	// 	logger.DebugContext(ctx, "updating an existing object")
	// 	// newInventory := &Inventory{}
	// 	if commit.ID != "" && currentID != commit.ID {
	// 		err = fmt.Errorf("object at %q has id %q, not the id given to commit: %q", obj.Path(), currentID, currentID)
	// 		return &CommitError{Err: err}
	// 	}
	// 	// changing digests algorithms isn't supported
	// 	if currentAlg != commit.Stage.DigestAlgorithm {
	// 		err := fmt.Errorf("object's digest algorithm (%s) doesn't match stage's (%s)", currentAlg, commit.Stage.DigestAlgorithm)
	// 		return &CommitError{Err: err}
	// 	}
	// 	lastVersion := currentInventory.Version(0)
	// 	if lastVersion == nil || lastVersion.State == nil {
	// 		// an error here indicates a bug in the inventory's validation during GetObject()
	// 		err = errors.New("existing object inventory doesn't include a valid version state")
	// 		return &CommitError{Err: err}
	// 	}
	// 	if !commit.AllowUnchanged && lastVersion.State().Eq(commit.Stage.State) {
	// 		err = fmt.Errorf("new version would have same state as existing version")
	// 		return &CommitError{Err: err}
	// 	}
	// 	newInventory.ID = currentID
	// 	newInventory.DigestAlgorithm = currentAlg
	// 	newInventory.ContentDirectory = currentInventory.ContentDirectory()
	// 	newInventory.Head, err = currentInventory.Head().Next()
	// 	if err != nil {
	// 		err = fmt.Errorf("inventory's version scheme doesn't allow additional versions: %w", err)
	// 		return &CommitError{Err: err}
	// 	}
	// default: // new object
	// 	logger.DebugContext(ctx, "commiting new object")
	// 	// create base inventory with '0' head version:
	// 	// the head is incremented later by inv.NextInventory
	// 	newInventory.ID = commit.ID
	// 	newInventory.DigestAlgorithm = commit.Stage.DigestAlgorithm
	// 	newInventory.ContentDirectory = contentDir
	// 	newInventory.Head = ocfl.V(1, 0)
	// }
	// Determine the OCFL spec to use for the new inventory if none given.
	// var newSpec = opts.spec
	// if newSpec.Empty() {
	// 	switch {
	// 	case !opts.storeSpec.Empty():
	// 		// use the storage root's spec, if available.
	// 		newSpec = opts.storeSpec
	// 	case obj.Exists():
	// 		// use the existing object's spec, if available.
	// 		newSpec = obj.Inventory().Spec()
	// 	// otherwise, default spec
	// 	default:
	// 		newSpec = defaultSpec
	// 	}
	// }
	// check that the ocfl spec is valid:
	// - storage root's spec is a max
	// - existing object spec is a min
	// if !opts.storeSpec.Empty() && newSpec.Cmp(opts.storeSpec) > 0 {
	// 	err = fmt.Errorf("new object version's OCFL spec can't be higher than the storage root's (%s)", opts.storeSpec)
	// 	return &CommitError{Err: err}
	// }
	// if obj.Exists() && newSpec.Cmp(obj.Inventory().Spec()) < 0 {
	// 	err = fmt.Errorf("new object version's OCFL spec can't be lower than the current version (%s)", obj.Inventory().Spec())
	// 	return &CommitError{Err: err}
	// }
	// baseInv.Type = newSpec.AsInvType()

	// // check requiredHEAD constraint
	// if commit.NewHEAD > 0 && newHead != commit.NewHEAD {
	// 	err = fmt.Errorf("commit is constrained to version number %d, but the object's next version should have number %d",
	// 		opts.requireHEAD, newHead)
	// 	return &CommitError{Err: err}
	// }

	// build new inventory

	// this check is reduntant given previous validations, but we don't want to
	// over-write an existing version directory.
	// if existObj != nil && slices.Contains(existObj.ObjectRoot.State.VersionDirs, newInv.Head) {
	// 	err = fmt.Errorf("version directory %q already exists in %q", newInv.Head, objPath)
	// 	return &CommitError{Err: err}
	// }
	logger = logger.With("head", newInv.Head, "ocfl_spec", newInv.Type.Spec, "alg", newInv.DigestAlgorithm)

	// xfers is a subeset of the manifest with new content to add
	xfers, err := xferMap(newInv)
	if err != nil {
		return &CommitError{Err: err}
	}
	if len(xfers) > 0 && commit.Stage.ContentSource == nil {
		err := errors.New("stage is missing a source for new content")
		return &CommitError{Err: err}
	}
	// check that the stage's content source provides all new content
	for digest := range xfers {
		if !commit.Stage.HasContent(digest) {
			err := fmt.Errorf("stage's content source can't provide digest: %s", digest)
			return &CommitError{Err: err}
		}
	}

	// File changes start here
	// TODO: replace existing object declaration if spec is changing

	// Mutate object: new object declaration if necessary
	if !obj.Exists() {
		logger.DebugContext(ctx, "initializing new OCFL object")
		decl := ocfl.Namaste{Type: ocfl.NamasteTypeObject, Version: newInv.Type.Spec}
		if err = ocfl.WriteDeclaration(ctx, writeFS, obj.Path(), decl); err != nil {
			return &CommitError{Err: err, Dirty: true}
		}
	}

	// Mutate object: tranfser files from stage to object
	if len(xfers) > 0 {
		xferOpts := &commitCopyOpts{
			Source:   commit.Stage,
			DestFS:   writeFS,
			DestRoot: obj.Path(),
			Manifest: xfers,
		}
		if err = commitCopy(ctx, xferOpts); err != nil {
			err = fmt.Errorf("transferring new object contents: %w", err)
			return &CommitError{Err: err, Dirty: true}
		}
	}

	logger.DebugContext(ctx, "writing inventories for new object version")

	// Mutate object: write inventory to both object root and version directory
	newVDir := path.Join(obj.Path(), newInv.Head.String())
	if err = WriteInventory(ctx, writeFS, newInv, obj.Path(), newVDir); err != nil {
		err = fmt.Errorf("writing new inventories or inventory sidecars: %w", err)
		return &CommitError{Err: err, Dirty: true}
	}
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

// xferMap returns a DigestMap that is a subset of the inventory
// manifest for the digests and paths of new content
func xferMap(inv *RawInventory) (ocfl.DigestMap, error) {
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
