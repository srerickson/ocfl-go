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

// CommitOption is used configure Commit
type CommitOption func(*commit)

// WithOCFLSpec is used to set the OCFL specification for the new object
// version.
func WithOCFLSpec(spec ocfl.Spec) CommitOption {
	return func(comm *commit) {
		comm.spec = spec
	}
}

// WithContentDir is used to set the ContentDirectory value for the first
// version of an object. It is ignored for subsequent versions.
func WithContentDir(cd string) CommitOption {
	return func(comm *commit) {
		comm.contentDir = cd
	}
}

// WithVersionPadding is used to set the version number padding for the first
// version of an object. It is ignored for subsequent versions.
func WithVersionPadding(p int) CommitOption {
	return func(comm *commit) {
		comm.padding = p
	}
}

// WithVersion is used to enforce a particul version number for the commit.
// The default is to increment the existing verion if possible.
func WithVersion(v int) CommitOption {
	return func(comm *commit) {
		comm.requireV = v
	}
}

// WithContentPathFunc is a functional option used to set the stage's content path
// function.
func WithContentPathFunc(fn ContentPathFunc) CommitOption {
	return func(comm *commit) {
		comm.contentPathFunc = fn
	}
}

// WithMessage sets the message for the new object version
func WithMessage(msg string) CommitOption {
	return func(comm *commit) {
		comm.message = msg
	}
}

// WithUser sets the user for the new object version
func WithUser(user User) CommitOption {
	return func(comm *commit) {
		comm.user = &user
	}
}

// WithCreated sets the created timestamp for the new object version to
// a non-default value. The default is
func WithCreated(c time.Time) CommitOption {
	return func(comm *commit) {
		comm.created = c
	}
}

func WithLogger(logger logr.Logger) CommitOption {
	return func(comm *commit) {
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

// commit represents an OCFL object transformation
type commit struct {
	storeFS ocfl.WriteFS
	objRoot string      // object root
	stage   *ocfl.Stage // content to commit
	newInv  *Inventory  // inventory to commit

	// options
	requireV        int             // new inventory must have this version number (if non-zero)
	spec            ocfl.Spec       // OCFL spec for new version
	padding         int             // padding (new objects only)
	contentDir      string          // content directory setting (new objects only)
	contentPathFunc ContentPathFunc // function used to configure content paths
	user            *User
	message         string
	created         time.Time

	logger logr.Logger
}

// commit performs the commit
func (comm *commit) commit(ctx context.Context) error {
	id := comm.newInv.ID
	vnum := comm.newInv.Head
	xfers, err := comm.transferMap()
	if err != nil {
		return fmt.Errorf("commit canceled: %w", err)
	}
	stageFS, _ := comm.stage.Root()
	if len(xfers) > 0 && stageFS == nil {
		return fmt.Errorf("commit canceled: stage is missing an FS")
	}
	comm.logger.Info("starting commit", "object_id", id, "head", vnum)
	defer comm.logger.Info("commit complete", "object_id", id, "head", vnum)
	if vnum.First() {
		// for v1, expect version directory to ErrNotExist or be empty
		entries, err := comm.storeFS.ReadDir(ctx, comm.objRoot)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("commit canceled: %w", err)
		}
		if len(entries) > 0 {
			return errors.New("commit canceled: object directory is not empty")
		}
	} else {
		// for v > 1, the version directory must not exist or be empty
		entries, err := comm.storeFS.ReadDir(ctx, path.Join(comm.objRoot, vnum.String()))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("commit canceled: %w", err)
		}
		if len(entries) > 0 {
			return fmt.Errorf("commit canceled: version directory '%s' not empty", vnum.String())
		}
	}
	// write declaration for first version
	// TODO: replace Namaste if new inventory uses newew spec
	if vnum.First() {
		decl := ocfl.Declaration{
			Type:    ocfl.DeclObject,
			Version: comm.newInv.Type.Spec,
		}
		if err := ocfl.WriteDeclaration(ctx, comm.storeFS, comm.objRoot, decl); err != nil {
			return fmt.Errorf("writing object declaration: %w", err)
		}
	}
	// tranfser files from stage to object
	// TODO: set concurrency with commit option
	if err := xfer.Copy(ctx, stageFS, comm.storeFS, xfers, runtime.NumCPU()); err != nil {
		return fmt.Errorf("transfering new object contents: %w", err)
	}
	// write inventory to both object root and version directory
	vDir := path.Join(comm.objRoot, vnum.String())
	if err := WriteInventory(ctx, comm.storeFS, comm.newInv, comm.objRoot, vDir); err != nil {
		return fmt.Errorf("writing new inventories: %w", err)
	}
	return nil
}

// transferMap builds a map of destination/source paths representing
// file to copy from the stage to the object root. Source paths
// are relative to the stage's FS. Destination paths are relative to
// storage root's FS
func (comm *commit) transferMap() (map[string]string, error) {
	stageMan, err := comm.stage.Manifest()
	if err != nil {
		return nil, fmt.Errorf("stage has errors: %w", err)
	}
	inv := comm.newInv
	if inv == nil || inv.Manifest == nil {
		return nil, errors.New("stage is not complete: missing inventory manifest")
	}
	_, stageRoot := comm.stage.Root()
	objRoot := comm.objRoot
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
