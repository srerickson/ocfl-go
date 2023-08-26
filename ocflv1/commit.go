package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/internal/xfer"
	"github.com/srerickson/ocfl/validation"
	"golang.org/x/exp/slog"
)

// Commit creates or updates an object in fsys using the provided stage state
// and content.
func Commit(ctx context.Context, fsys ocfl.WriteFS, objRoot string, id string, stage *ocfl.Stage, optFuncs ...CommitOption) error {
	// default commit options
	opts := &commitOpt{
		created:    time.Now().UTC().Truncate(time.Second),
		contentDir: contentDir,
	}
	// load options
	for _, optFunc := range optFuncs {
		optFunc(opts)
	}
	updating := false  // update or creating?
	var inv *Inventory // new inventory
	// read existing inventory
	invFile, openErr := fsys.OpenFile(ctx, path.Join(objRoot, inventoryFile))
	if openErr == nil {
		// updating existing object
		defer invFile.Close()
		updating = true
		var vErr *validation.Result
		inv, vErr = ValidateInventoryReader(ctx, invFile, nil, ValidationLogger(opts.logger))
		if err := vErr.Err(); err != nil {
			err = fmt.Errorf("existing inventory is invalid: %w", err)
			return &CommitError{Err: err}
		}
		if inv.ID != id {
			err := fmt.Errorf("existing Object ID (%q) doesn't match ID given to Commit (%q)", inv.ID, id)
			return &CommitError{Err: err}
		}
		invState, err := inv.objectState(0)
		if err != nil {
			// inventory should be valid
			err := fmt.Errorf("can't create object state with existing inventory (this is a bug): %w", err)
			return &CommitError{Err: err}
		}
		if !opts.allowUnchanged && invState.Eq(stage.State()) {
			err := fmt.Errorf("new version would have same state as existing version")
			return &CommitError{Err: err}
		}
	}
	if openErr != nil {
		if !errors.Is(openErr, fs.ErrNotExist) {
			return &CommitError{Err: openErr}
		}
		// creating new object
		if stage.Alg == nil {
			err := errors.New("stage algorith required for new objects")
			return &CommitError{Err: err}
		}
		// create new inventory with '0' head version:
		// the head is incremented later by inv.AddVersion
		inv = &Inventory{
			ID:               id,
			Head:             ocfl.V(0, opts.padding),
			DigestAlgorithm:  stage.Alg.ID(),
			ContentDirectory: opts.contentDir,
		}
	}
	// Determine the spec to use for new inventory if no spec option was given.
	var spec = opts.spec
	if spec.Empty() {
		switch {
		// use  the storage root's spec, if available
		case !opts.storeSpec.Empty():
			spec = opts.storeSpec
		// use the existing object's spec, if available.
		case updating:
			spec = inv.Type.Spec
		// otherwise, default spec
		default:
			spec = defaultSpec
		}
	}
	// check that the spec valid:
	// - storage root's spec is a max
	// - existing object spec is a min
	if !opts.storeSpec.Empty() && spec.Cmp(opts.storeSpec) > 0 {
		err := fmt.Errorf("object's OCFL spec can't be set higher than the storage root's (%s)", opts.storeSpec)
		return &CommitError{Err: err}
	}
	if updating && spec.Cmp(inv.Type.Spec) < 0 {
		err := fmt.Errorf("object's OCFL spec can't be set lower than the existing value (%s)", inv.Type.Spec)
		return &CommitError{Err: err}
	}
	inv.Type = spec.AsInvType()
	// update inventory with state from stage
	if err := inv.AddVersion(stage, opts.message, opts.user, opts.created, opts.pathFn); err != nil {
		return &CommitError{Err: err}
	}
	return commit(ctx, fsys, objRoot, inv, stage, opts)
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
// version.
func WithOCFLSpec(spec ocfl.Spec) CommitOption {
	return func(comm *commitOpt) {
		comm.spec = spec
	}
}

// WithContentDir is used to set the ContentDirectory value for the first
// version of an object. It is ignored for subsequent versions.
func WithContentDir(cd string) CommitOption {
	return func(comm *commitOpt) {
		comm.contentDir = cd
	}
}

// WithVersionPadding is used to set the version number padding for the first
// version of an object. It is ignored for subsequent versions.
func WithVersionPadding(p int) CommitOption {
	return func(comm *commitOpt) {
		comm.padding = p
	}
}

// WithHEAD is used to enforce a particul version number for the commit.
// The default is to increment the existing verion if possible.
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

// WithUser sets the user for the new object version
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

// commit performs the commit
func commit(ctx context.Context, fsys ocfl.WriteFS, objRoot string, inv *Inventory, stage *ocfl.Stage, opts *commitOpt) error {
	id := inv.ID
	vnum := inv.Head
	if opts.requireHEAD > 0 && inv.Head.Num() != opts.requireHEAD {
		err := fmt.Errorf("cannot commit HEAD=%d (next HEAD=%d)", opts.requireHEAD, inv.Head.Num())
		return &CommitError{Err: err}
	}
	xfers, err := transferMap(stage, inv, objRoot)
	if err != nil {
		return &CommitError{Err: err}
	}
	if opts.logger != nil {
		opts.logger.Info("starting commit", "object_id", id, "head", vnum)
		defer opts.logger.Info("commit complete", "object_id", id, "head", vnum)
	}
	// init object root: it returns both obj and error if the object already exists.
	obj, err := ocfl.InitObjectRoot(ctx, fsys, objRoot, inv.Type.Spec)
	if err != nil {
		if !errors.Is(err, ocfl.ErrObjectExists) {
			return &CommitError{Err: fmt.Errorf("initializing object root: %w", err), Dirty: true}
		}
		// existing object root
		if obj != nil && obj.Spec.Cmp(inv.Type.Spec) > 0 {
			err := fmt.Errorf("existing object declaration has higher OCFL spec than inventory (v%s > v%s)", obj.Spec, inv.Type.Spec)
			return &CommitError{Err: err}
		}
		// TODO: upgrade object declaration.
		if obj != nil && obj.Spec.Cmp(inv.Type.Spec) < 0 {
			err := fmt.Errorf("upgrading existing object declaration from OCFL v%s to %s is not yet supported", obj.Spec, inv.Type.Spec)
			return &CommitError{Err: err}
		}
	}
	// don't overwrite any existing content directories
	if obj.HasVersionDir(inv.Head) {
		err := fmt.Errorf("a directory for '%s' already exists", inv.Head.String())
		return &CommitError{Err: err}
	}
	// tranfser files from stage to object
	// TODO: set concurrency with commit option
	if len(xfers) > 0 {
		if err := xfer.Copy(ctx, stage.FS, fsys, xfers, runtime.NumCPU()); err != nil {
			return &CommitError{
				Err:   fmt.Errorf("transfering new object contents: %w", err),
				Dirty: true,
			}
		}
	}

	// write inventory to both object root and version directory
	vDir := path.Join(objRoot, vnum.String())
	if err := WriteInventory(ctx, fsys, inv, objRoot, vDir); err != nil {
		return &CommitError{
			Err:   fmt.Errorf("writing new inventories or inventory sidecars: %w", err),
			Dirty: true,
		}
	}
	return nil
}

// transferMap builds a map of destination/source paths representing
// file to copy from the stage to the object root. Source paths
// are relative to the stage's FS. Destination paths are relative to
// storage root's FS
func transferMap(stage *ocfl.Stage, inv *Inventory, objRoot string) (map[string]string, error) {
	xfer := map[string]string{}
	for dst, dig := range inv.Manifest.AllPaths() {
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
