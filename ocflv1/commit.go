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

	"github.com/go-logr/logr"
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/internal/xfer"
	"github.com/srerickson/ocfl/validation"
)

func Commit(ctx context.Context, fsys ocfl.WriteFS, objRoot string, id string, stage *ocfl.Stage, optFuncs ...CommitOption) error {
	// defaults
	opts := &commitOpt{
		spec:       defaultSpec,
		logger:     logr.Discard(),
		created:    time.Now().UTC().Truncate(time.Second),
		contentDir: contentDir,
	}
	// load options
	for _, optFunc := range optFuncs {
		optFunc(opts)
	}
	var inv *Inventory
	// read existing inventory
	invFile, openErr := fsys.OpenFile(ctx, path.Join(objRoot, inventoryFile))
	if openErr == nil {
		// updating existing object
		defer invFile.Close()
		var vErr *validation.Result
		inv, vErr = ValidateInventoryReader(ctx, invFile, nil, ValidationLogger(opts.logger))
		if err := vErr.Err(); err != nil {
			err = fmt.Errorf("existing inventory is invalid: %w", err)
			return &CommitError{Err: err}
		}
		if inv.ID != id {
			err := fmt.Errorf("existing inventory ID ('%s') doesn't match ID given to Commit ('%s')", inv.ID, id)
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
		inv = &Inventory{
			ID:               id,
			Head:             ocfl.V(0, opts.padding), // head incremented by inv.AddVersion
			Type:             opts.spec.AsInvType(),
			DigestAlgorithm:  stage.Alg.ID(),
			ContentDirectory: opts.contentDir,
		}
	}
	// apply the staged changes to the inventory
	if err := inv.addVersion(stage, opts.pathFn, opts.created, opts.message, opts.user); err != nil {
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
	requireHEAD int                             // new inventory must have this version number (if non-zero)
	spec        ocfl.Spec                       // OCFL spec for new version
	padding     int                             // padding (new objects only)
	contentDir  string                          // inventory's content directory setting (new objects only)
	user        *User                           // inventory's version state user
	message     string                          // inventory's version state message
	created     time.Time                       // inventory's version state created value
	pathFn      func(string, []string) []string // function to tranform staged content paths
	logger      logr.Logger
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

func WithLogger(logger logr.Logger) CommitOption {
	return func(comm *commitOpt) {
		comm.logger = logger
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
	opts.logger.Info("starting commit", "object_id", id, "head", vnum)
	defer opts.logger.Info("commit complete", "object_id", id, "head", vnum)
	// init object root
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
		sources := stage.ContentPaths(dig)
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
