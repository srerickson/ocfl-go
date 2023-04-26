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
)

func Commit(ctx context.Context, fsys ocfl.WriteFS, objRoot string, id string, stage *ocfl.Stage, optFuncs ...CommitOption) error {
	// defaults
	opts := &commitOpt{
		spec:            defaultSpec,
		contentPathFunc: DefaultContentPathFunc,
		logger:          logr.Discard(),
		created:         time.Now().UTC().Truncate(time.Second),
	}
	// load options
	for _, optFunc := range optFuncs {
		optFunc(opts)
	}
	obj, err := GetObject(ctx, fsys, objRoot)
	if err != nil && !errors.Is(err, ocfl.ErrObjectNotFound) {
		return &CommitError{Err: err}
	}
	// new object
	if obj == nil {
		inv, err := NewInventory(stage, id, opts.contentDir, opts.padding, opts.created, opts.message, opts.user)
		if err != nil {
			return &CommitError{Err: fmt.Errorf("while building new inventory: %w", err)}
		}
		return commit(ctx, fsys, objRoot, inv, stage, opts)
	}
	// update object
	prevInv, err := obj.Inventory(ctx)
	if err != nil {
		return &CommitError{Err: err}
	}
	inv, err := prevInv.NextVersionInventory(stage, opts.created, opts.message, opts.user)
	if err != nil {
		return &CommitError{Err: fmt.Errorf("while building next inventory: %w", err)}
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
	requireHEAD     int             // new inventory must have this version number (if non-zero)
	spec            ocfl.Spec       // OCFL spec for new version
	padding         int             // padding (new objects only)
	contentPathFunc ContentPathFunc // function used to configure content paths
	contentDir      string          // inventory'ßs content directory setting (new objects only)
	user            *User           // inventory's version state user
	message         string          // inventory's version state message
	created         time.Time       // inventory's version state created value
	logger          logr.Logger
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

// WithContentPathFunc is a functional option used to set the stage's content path
// function.
func WithContentPathFunc(fn ContentPathFunc) CommitOption {
	return func(comm *commitOpt) {
		comm.contentPathFunc = fn
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

func WithLogger(logger logr.Logger) CommitOption {
	return func(comm *commitOpt) {
		comm.logger = logger
	}
}

// ContentPathFunc is a function used to determin the path for content
// file in an OCFL object version.
type ContentPathFunc func(logical string, digest string) string

// DefaultContentPathFunc is the default ContentPathFunc. It returns
// logical
func DefaultContentPathFunc(logical string, digest string) string {
	return logical
}

// commit performs the commit
func commit(ctx context.Context, fsys ocfl.WriteFS, objRoot string, inv *Inventory, stage *ocfl.Stage, opts *commitOpt) error {
	id := inv.ID
	vnum := inv.Head
	if opts.requireHEAD > 0 && inv.Head.Num() != opts.requireHEAD {
		err := fmt.Errorf("cannot commit HEAD=%d (next HEAD=%d)", opts.requireHEAD, inv.Head.Num())
		return &CommitError{Err: err}
	}
	stageFS, stageRoot := stage.Root()
	xfers, err := transferMap(stage, inv, stageRoot, objRoot)
	if err != nil {
		return &CommitError{Err: err}
	}
	if len(xfers) > 0 && stageFS == nil {
		err := errors.New("stage is not configured with a backing FS")
		return &CommitError{Err: err}
	}
	opts.logger.Info("starting commit", "object_id", id, "head", vnum)
	defer opts.logger.Info("commit complete", "object_id", id, "head", vnum)
	if vnum.First() {
		// for v1, expect version directory to ErrNotExist or be empty
		entries, err := fsys.ReadDir(ctx, objRoot)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return &CommitError{Err: err}
		}
		if len(entries) > 0 {
			err := errors.New("commit canceled: object directory is not empty")
			return &CommitError{Err: err}
		}
	} else {
		// for v > 1, the version directory must not exist or be empty
		entries, err := fsys.ReadDir(ctx, path.Join(objRoot, vnum.String()))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return &CommitError{Err: err}
		}
		if len(entries) > 0 {
			err := fmt.Errorf("version directory '%s' not empty", vnum.String())
			return &CommitError{Err: err}
		}
	}
	// write declaration for first version
	// TODO: replace Namaste if new inventory uses newew spec
	if vnum.First() {
		decl := ocfl.Declaration{
			Type:    ocfl.DeclObject,
			Version: inv.Type.Spec,
		}
		if err := ocfl.WriteDeclaration(ctx, fsys, objRoot, decl); err != nil {
			return &CommitError{Err: err}
		}
	}
	// tranfser files from stage to object
	// TODO: set concurrency with commit option
	if err := xfer.Copy(ctx, stageFS, fsys, xfers, runtime.NumCPU()); err != nil {
		return &CommitError{
			Err:   fmt.Errorf("transfering new object contents: %w", err),
			Dirty: true,
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
func transferMap(stage *ocfl.Stage, inv *Inventory, stageRoot string, objRoot string) (map[string]string, error) {
	stageMan, err := stage.Manifest()
	if err != nil {
		return nil, fmt.Errorf("stage has errors: %w", err)
	}
	if inv == nil || inv.Manifest == nil {
		return nil, errors.New("stage is not complete: missing inventory manifest")
	}
	xfer := map[string]string{}
	for dst, dig := range inv.Manifest.AllPaths() {
		// ignore manifest entries from previous versions
		if !strings.HasPrefix(dst, inv.Head.String()+"/") {
			continue
		}
		sources := stageMan.DigestPaths(dig)
		if len(sources) == 0 {
			return nil, fmt.Errorf("no source file provided for digest: %s", dig)
		}
		// prefix src with stage's root directory
		src := path.Join(stageRoot, sources[0])
		// prefix dst with object's root directory
		dst = path.Join(objRoot, dst)
		xfer[dst] = src
	}
	return xfer, nil
}
