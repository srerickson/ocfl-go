package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"slices"
	"strings"
	"time"

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
func Commit(ctx context.Context, fsys ocfl.WriteFS, objPath string, objID string, stage *ocfl.Stage, optFuncs ...CommitOption) (err error) {
	var (
		newHead  = 1           // new version num (no padding)
		baseInv  *Inventory    // existing/based inventory
		existObj *Object       // existing object
		opts     = &commitOpt{ // default opts
			created:    time.Now().UTC(),
			contentDir: contentDir,
			logger:     logging.DisabledLogger(),
		}
	)

	for _, optFunc := range optFuncs {
		optFunc(opts)
	}
	opts.created = opts.created.Truncate(time.Second)
	opts.logger = opts.logger.With("object_path", objPath, "object_id", objID)

	if stage.State == nil {
		stage.State = ocfl.DigestMap{}
	}

	existObj, err = GetObject(ctx, fsys, objPath)
	if err != nil {
		// Handle acceptable error from GetObject() if the object doesn't exist. For a
		// new object, the object path must not exist. The only acceptable error
		// here is ErrNotExist for the object path.
		var pathErr *fs.PathError
		if errors.Is(err, fs.ErrNotExist) && errors.As(err, &pathErr) && pathErr.Path == objPath {
			err = nil
		}
		if err != nil {
			return &CommitError{Err: err}
		}
	}
	switch {
	case existObj != nil:
		// Handle existing object
		opts.logger.DebugContext(ctx, "updating an existing object")
		baseInv = &existObj.Inventory
		newHead = baseInv.Head.Num() + 1
		if baseInv.ID != objID {
			err = fmt.Errorf("object at %q has id %q, not the id given to commit: %q", objPath, baseInv.ID, objID)
			return &CommitError{Err: err}
		}
		// changing digests algorithms isn't supported
		if baseInv.DigestAlgorithm != stage.DigestAlgorithm {
			err := fmt.Errorf("object's digest algorithm (%s) doesn't match stage's (%s)", baseInv.DigestAlgorithm, stage.DigestAlgorithm)
			return &CommitError{Err: err}
		}
		lastVersion := baseInv.Version(0)
		if lastVersion == nil || lastVersion.State == nil {
			// an error here indicates a bug in the inventory's validation during GetObject()
			err = errors.New("existing object inventory doesn't include a valid version state")
			return &CommitError{Err: err}
		}
		if !opts.allowUnchanged && lastVersion.State.Eq(stage.State) {
			err = fmt.Errorf("new version would have same state as existing version")
			return &CommitError{Err: err}
		}
	default: // existObj == nil
		opts.logger.DebugContext(ctx, "commiting new object")
		// create base inventory with '0' head version:
		// the head is incremented later by inv.NextInventory
		baseInv = &Inventory{
			ID:               objID,
			Head:             ocfl.V(0, opts.padding),
			DigestAlgorithm:  stage.DigestAlgorithm,
			ContentDirectory: opts.contentDir,
		}
	}
	// Determine the OCFL spec to use for the new inventory if none given.
	var newSpec = opts.spec
	if newSpec.Empty() {
		switch {
		// use the storage root's spec, if available.
		case !opts.storeSpec.Empty():
			newSpec = opts.storeSpec
		// use the existing object's spec, if available.
		case existObj != nil:
			newSpec = existObj.State.Spec
		// otherwise, default spec
		default:
			newSpec = defaultSpec
		}
	}
	// check that the ocfl spec is valid:
	// - storage root's spec is a max
	// - existing object spec is a min
	if !opts.storeSpec.Empty() && newSpec.Cmp(opts.storeSpec) > 0 {
		err = fmt.Errorf("new object version's OCFL spec can't be higher than the storage root's (%s)", opts.storeSpec)
		return &CommitError{Err: err}
	}
	if existObj != nil && newSpec.Cmp(existObj.State.Spec) < 0 {
		err = fmt.Errorf("new object version's OCFL spec can't be lower than the current version (%s)", existObj.State.Spec)
		return &CommitError{Err: err}
	}
	baseInv.Type = newSpec.AsInvType()

	// check requiredHEAD constraint
	if opts.requireHEAD > 0 && newHead != opts.requireHEAD {
		err = fmt.Errorf("commit is constrained to version number %d, but the object's next version should have number %d",
			opts.requireHEAD, newHead)
		return &CommitError{Err: err}
	}

	// build new inventory
	newVersion := &Version{
		State:   stage.State,
		Message: opts.message,
		User:    opts.user,
		Created: opts.created,
	}
	newInv, err := NewInventory(baseInv, newVersion, stage.FixitySource, opts.pathFn)
	if err != nil {
		err := fmt.Errorf("building new inventory: %w", err)
		return &CommitError{Err: err}
	}

	// this check is reduntant given previous validations, but we don't want to
	// over-write an existing version directory.
	if existObj != nil && slices.Contains(existObj.ObjectRoot.State.VersionDirs, newInv.Head) {
		err = fmt.Errorf("version directory %q already exists in %q", newInv.Head, objPath)
		return &CommitError{Err: err}
	}
	opts.logger = opts.logger.With("head", newInv.Head, "ocfl_spec", newInv.Type.Spec, "alg", newInv.DigestAlgorithm)

	// xfers is a subeset of the manifest with new content to add
	xfers, err := xferMap(newInv)
	if err != nil {
		return &CommitError{Err: err}
	}
	if len(xfers) > 0 && stage.ContentSource == nil {
		err := errors.New("stage is missing a source for new content")
		return &CommitError{Err: err}
	}
	// check that the stage's content source provides all new content
	for digest := range xfers {
		if !stage.HasContent(digest) {
			err := fmt.Errorf("stage's content source can't provide digest: %s", digest)
			return &CommitError{Err: err}
		}
	}

	// File changes start here
	// TODO: replace existing object declaration if spec is changing

	// Mutate object: new object declaration if necessary
	if existObj == nil {
		opts.logger.DebugContext(ctx, "initializing new OCFL object")
		decl := ocfl.Namaste{Type: ocfl.NamasteTypeObject, Version: newInv.Type.Spec}
		if err = ocfl.WriteDeclaration(ctx, fsys, objPath, decl); err != nil {
			return &CommitError{Err: err, Dirty: true}
		}
	}

	// Mutate object: tranfser files from stage to object
	if len(xfers) > 0 {
		xferOpts := &commitCopyOpts{
			Source:   stage,
			DestFS:   fsys,
			DestRoot: objPath,
			Manifest: xfers,
		}
		if err = commitCopy(ctx, xferOpts); err != nil {
			err = fmt.Errorf("transferring new object contents: %w", err)
			return &CommitError{Err: err, Dirty: true}
		}
	}

	opts.logger.DebugContext(ctx, "writing inventories for new object version")

	// Mutate object: write inventory to both object root and version directory
	newVDir := path.Join(objPath, newInv.Head.String())
	if err = WriteInventory(ctx, fsys, newInv, objPath, newVDir); err != nil {
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

// commitOpt is the internal struct for commit options configured
// using one of the CommitOptions
type commitOpt struct {
	requireHEAD    int        // new inventory must have this version number (if non-zero)
	spec           ocfl.Spec  // OCFL spec for new version
	storeSpec      ocfl.Spec  // OCFL spec from storage root (used internally)
	user           *ocfl.User // inventory's version state user
	message        string     // inventory's version state message
	created        time.Time  // inventory's version state created value
	allowUnchanged bool       // allow new versions with same state as previous version
	contentDir     string     // inventory's content directory setting (new objects only)
	padding        int        // padding (new objects only)

	pathFn func([]string) []string // function to transform paths in stage state
	logger *slog.Logger
}

// CommitOption is used configure Commit
type CommitOption func(*commitOpt)

// WithOCFLSpec is used to set the OCFL specification for the new object
// version. The spec version cannot be higher than the object's storage root
// and it cannot be lower the existing object version (if updating).
func WithOCFLSpec(spec ocfl.Spec) CommitOption {
	return func(comm *commitOpt) {
		comm.spec = spec
	}
}

// WithContentDir is used to set the contentDirectory when creating objects.
// This option is ignored for object updates.
func WithContentDir(cd string) CommitOption {
	return func(comm *commitOpt) {
		comm.contentDir = cd
	}
}

// WithVersionPadding is used to set the version number padding when creating
// objects. The padding is the maximum number of numeric digits the version
// number can include (a padding of 0 is no maximum). This option is ignored for
// object updates.
func WithVersionPadding(p int) CommitOption {
	return func(comm *commitOpt) {
		comm.padding = p
	}
}

// WithHEAD is used to constrain the version number for the commit. For example,
// WithHEAD(1) can be used to cause a commit to fail if the object already
// exits.
func WithHEAD(v int) CommitOption {
	return func(comm *commitOpt) {
		comm.requireHEAD = v
	}
}

// WithMessage sets the message for the new object version
func WithMessage(msg string) CommitOption {
	return func(comm *commitOpt) {
		comm.message = msg
	}
}

// WithUser sets the user for the new object version.
func WithUser(user *ocfl.User) CommitOption {
	return func(comm *commitOpt) {
		comm.user = user
	}
}

// WithCreated sets the created timestamp for the new object version to
// a non-default value. The default is
func WithCreated(c time.Time) CommitOption {
	return func(comm *commitOpt) {
		comm.created = c
	}
}

// WitManifestPathFunc is a function used to configure paths for content
// files saved to the object with the commit. The function is called for each
// new manifest entry (digest/path list); The function should
// return a slice of paths indicating where the content should be saved
// (relative) object version's content directory.
func WithManifestPathFunc(fn func(paths []string) []string) CommitOption {
	return func(comm *commitOpt) {
		comm.pathFn = fn
	}
}

// WithLogger sets the logger used for logging during commit.
func WithLogger(logger *slog.Logger) CommitOption {
	return func(comm *commitOpt) {
		comm.logger = logger
	}
}

// WithAllowUnchanged enables committing a version with the same state
// as the existing head version.
func WithAllowUnchanged(val bool) CommitOption {
	return func(comm *commitOpt) {
		comm.allowUnchanged = val
	}
}

// withStoreSpec is used by Store.Commit() to pass the storage
// root's OCFL spec to Commit()
func withStoreSpec(spec ocfl.Spec) CommitOption {
	return func(comm *commitOpt) {
		comm.storeSpec = spec
	}
}

// xferMap returns a DigestMap that is a subset of the inventory
// manifest for the digests and paths of new content
func xferMap(inv *Inventory) (ocfl.DigestMap, error) {
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
