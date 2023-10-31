package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/internal/xfer"
	"github.com/srerickson/ocfl-go/logging"
	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
)

// Commit creates or updates the OCFL v1.x object objectID at objPath using the
// content from stage. If an object exists at objPath, the object will be
// updated; if objPath does not exist, a new object will be created. If objPath
// exists but it is not the root of a valid OCFL object, an error is returned.
// The digest algorithm for the object version is taken from the stage. For
// object updates, the stage's algorithm must match the existing object's digest
// algorithm. The error returned by commit is always a CommitError.
func Commit(ctx context.Context, fsys ocfl.WriteFS, objPath string, objID string, stage *ocfl.Stage, optFuncs ...CommitOption) (err error) {
	// default commit options
	opts := &commitOpt{
		created:    time.Now().UTC(),
		contentDir: contentDir,
		logger:     logging.DisabledLogger(),
	}
	for _, optFunc := range optFuncs {
		optFunc(opts)
	}
	opts.created = opts.created.Truncate(time.Second)
	opts.logger = opts.logger.With("object_path", objPath, "object_id", objID)

	// Start by generating the new inventory: from existing or from scratch.

	var newInv *Inventory                             // new inventory
	existing, objErr := GetObject(ctx, fsys, objPath) // existing object

	// Handle existing object: new inventory is based on normalized copy of
	// the existing inventory.
	if objErr == nil {
		opts.logger.DebugCtx(ctx, "updating an existing object")

		if foundID := existing.Inventory.ID; foundID != objID {
			err = fmt.Errorf("object at %q has id %q, not the id given to commit: %q", objPath, foundID, objID)
			return &CommitError{Err: err}
		}
		invState, err := existing.Inventory.objectState(0)
		if err != nil {
			// an error here indicates a bug in the inventory's validation during GetObject()
			err = fmt.Errorf("creating object state from existing inventory (this is probably a bug): %w", err)
			return &CommitError{Err: err}
		}
		if !opts.allowUnchanged && invState.Eq(stage.State) {
			err = fmt.Errorf("new version would have same state as existing version")
			return &CommitError{Err: err}
		}
		newInv, err = existing.Inventory.NormalizedCopy()
		if err != nil {
			// an error here indicates a bug in the inventory's validation during GetObject()
			err = fmt.Errorf("building normalized copy of existing inventory: %w", err)
			return &CommitError{Err: err}
		}
	}

	// Handle possible error from GetObject() if the object doesn't exist. For a
	// new object, the object path must not exist. The only acceptable error
	// here is ErrNotExist for the object path.
	if objErr != nil {
		opts.logger.DebugCtx(ctx, "commiting new object")

		var pathErr *fs.PathError
		if errors.Is(objErr, fs.ErrNotExist) && errors.As(objErr, &pathErr) && pathErr.Path == objPath {
			objErr = nil
		}
		if objErr != nil {
			err = fmt.Errorf("path %q exists but it is not a valid object: %w", objPath, objErr)
			return &CommitError{Err: err}
		}
		// create new inventory with '0' head version:
		// the head is incremented later by inv.AddVersion
		newInv = &Inventory{
			ID:               objID,
			Head:             ocfl.V(0, opts.padding),
			DigestAlgorithm:  stage.Alg,
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
		case existing != nil:
			newSpec = existing.Spec
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
	if existing != nil && newSpec.Cmp(existing.Spec) < 0 {
		err = fmt.Errorf("new object version's OCFL spec can't be lower than the current version (%s)", existing.Spec)
		return &CommitError{Err: err}
	}
	newInv.Type = newSpec.AsInvType()

	// build new inventory from state
	err = newInv.AddVersion(stage, opts.message, opts.user, opts.created, opts.pathFn)
	if err != nil {
		return &CommitError{Err: err}
	}

	// check commit's required head constraint
	if n := newInv.Head.Num(); opts.requireHEAD > 0 && n != opts.requireHEAD {
		err = fmt.Errorf("commit is constrained to version number %d, but the object's next version should have number %d",
			opts.requireHEAD, n)
		return &CommitError{Err: err}
	}
	// this check is reduntant given previous validations, but we don't want to
	// over-write an existing version directory.
	if existing != nil && slices.Contains(existing.ObjectRoot.VersionDirs, newInv.Head) {
		err = fmt.Errorf("version directory %q already exists in %q", newInv.Head, objPath)
		return &CommitError{Err: err}
	}
	opts.logger = opts.logger.With("head", newInv.Head, "ocfl_spec", newInv.Type.Spec, "alg", newInv.DigestAlgorithm)
	// map of files to transfer
	xfers, err := xferMap(stage, newInv, objPath)
	if err != nil {
		return &CommitError{Err: err}
	}

	// File changes start here
	// TODO: replace existing object declaration if spec is changing

	// Mutate object: new object declaration if necessary
	if existing == nil {
		opts.logger.DebugCtx(ctx, "initializing new OCFL object")
		decl := ocfl.Declaration{Type: ocfl.DeclObject, Version: newInv.Type.Spec}
		if err = ocfl.WriteDeclaration(ctx, fsys, objPath, decl); err != nil {
			return &CommitError{Err: err, Dirty: true}
		}
	}

	// Mutate object: tranfser files from stage to object
	if len(xfers) > 0 {
		if err = xfer.Copy(ctx, stage.FS, fsys, xfers, ocfl.XferConcurrency(), opts.logger.WithGroup("xfer")); err != nil {
			err = fmt.Errorf("transferring new object contents: %w", err)
			return &CommitError{Err: err, Dirty: true}
		}
	}

	opts.logger.DebugCtx(ctx, "writing inventories for new object version")

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
	requireHEAD    int       // new inventory must have this version number (if non-zero)
	spec           ocfl.Spec // OCFL spec for new version
	storeSpec      ocfl.Spec // OCFL spec from storage root (used internally)
	user           *User     // inventory's version state user
	message        string    // inventory's version state message
	created        time.Time // inventory's version state created value
	allowUnchanged bool      // allow new versions with same state as previous version
	contentDir     string    // inventory's content directory setting (new objects only)
	padding        int       // padding (new objects only)

	pathFn func(string, []string) []string // function to transform paths in stage state
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
func WithUser(user User) CommitOption {
	return func(comm *commitOpt) {
		comm.user = &user
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
func WithManifestPathFunc(fn func(digest string, paths []string) []string) CommitOption {
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
func WithAllowUnchanged() CommitOption {
	return func(comm *commitOpt) {
		comm.allowUnchanged = true
	}
}

// withStoreSpec is used by Store.Commit() to pass the storage
// root's OCFL spec to Commit()
func withStoreSpec(spec ocfl.Spec) CommitOption {
	return func(comm *commitOpt) {
		comm.storeSpec = spec
	}
}

// xferMap builds a map of destination/source paths representing
// file to copy from the stage to the object root. Source paths
// are relative to the stage's FS. Destination paths are relative to
// storage root's FS
func xferMap(stage *ocfl.Stage, inv *Inventory, objRoot string) (map[string]string, error) {
	xfer := map[string]string{}
	for dst, dig := range inv.Manifest.PathMap() {
		// ignore manifest entries from previous versions
		if !strings.HasPrefix(dst, inv.Head.String()+"/") {
			continue
		}
		if stage.FS == nil {
			return nil, errors.New("missing staged content FS")
		}
		sources := stage.GetContent(dig)
		if len(sources) == 0 {
			return nil, fmt.Errorf("no source file provided for digest: %s", dig)
		}
		// prefix src with stage's root directory
		src := path.Join(stage.Root, sources[0])
		// prefix dst with object's root directory
		dst = path.Join(objRoot, dst)
		xfer[dst] = src
	}
	return xfer, nil
}
