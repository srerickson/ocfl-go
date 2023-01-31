package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/xfer"
)

// Commit creates or updates the object with the given id using the contents of stage.
func (s *Store) Commit(ctx context.Context, id string, stage *ocfl.Stage, opts ...CommitOption) error {
	s.commitLock.Lock()
	defer s.commitLock.Unlock()
	var prevInv *Inventory
	obj, err := s.GetObject(ctx, id)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("getting object info from storage root: %w", err)
	}
	if obj != nil {
		prevInv, err = obj.Inventory(ctx)
		if err != nil {
			return err
		}
	}
	// defaults
	comm := &commit{
		spec:            defaultSpec,
		contentPathFunc: DefaultContentPathFunc,
		stage:           stage,
		logger:          logr.Discard(),
	}
	// some defaults are based on the previous inventory
	if prevInv != nil {
		comm.spec = prevInv.Type.Spec
	}
	for _, opt := range opts {
		opt(comm)
	}
	var newInv *Inventory
	if prevInv != nil {
		newInv, err = prevInv.NextVersionInventory(stage, comm.spec, time.Now(), comm.message, &comm.user)
		if err != nil {
			return fmt.Errorf("while building next inventory")
		}
	} else {
		newInv, err = NewInventory(stage, id, comm.spec, comm.contentDir, comm.padding, time.Now(), comm.message, &comm.user)
		if err != nil {
			return fmt.Errorf("while building new inventory: %w", err)
		}
	}
	comm.newInv = newInv
	comm.manifest, err = stage.Manifest(nil)
	if err != nil {
		return fmt.Errorf("stage has errors: %w", err)
	}
	return s.commit(ctx, comm)
}

// commit represents an OCFL object transformation
type commit struct {
	stage    *ocfl.Stage // content to commit
	manifest *digest.Map // stage's manifest
	newInv   *Inventory  // inventory to commit

	// options
	requireV        int             // new inventory must have this version number (if non-zero)
	spec            ocfl.Spec       // OCFL spec for new version
	padding         int             // padding (new objects only)
	contentDir      string          // content directory setting (new objects only)
	contentPathFunc ContentPathFunc // function used to configure content paths
	user            User
	message         string
	nowrite         bool // used for "dry run" commit
	logger          logr.Logger
}

// commit creates or updates an object in the store using stage.
func (s *Store) commit(ctx context.Context, comm *commit) error {
	id := comm.newInv.ID
	vnum := comm.newInv.Head
	alg := comm.newInv.DigestAlgorithm
	writeFS, ok := s.fsys.(ocfl.WriteFS)
	if !ok {
		return fmt.Errorf("storage root backend is read-only")
	}
	if s.layoutFunc == nil {
		return fmt.Errorf("storage root layout must be set to commit: %w", ErrLayoutUndefined)
	}
	objPath, err := s.layoutFunc(id)
	if err != nil {
		return fmt.Errorf("object ID must be valid to commit: %w", err)
	}
	objPath = path.Join(s.rootDir, objPath)
	// file transfer list
	xfers, err := transferMap(comm.newInv, comm.manifest)
	if err != nil {
		return err
	}
	if fsys, _ := comm.stage.Root(); len(xfers) > 0 && fsys == nil {
		return fmt.Errorf("stage doesn't provide an FS for reading files to upload")
	}
	comm.logger.Info("committing new object version", "id", id, "head", vnum, "alg", alg, "message", comm.message)
	// expect version directory to ErrNotExist or be empty
	if vnum.First() {
		entries, err := s.fsys.ReadDir(ctx, objPath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if len(entries) != 0 {
			return errors.New("object directory must be empty to commit")
		}
	} else {
		entries, err := s.fsys.ReadDir(ctx, path.Join(objPath, vnum.String()))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("version directory '%s' must be empty to commit", vnum.String())
		}
	}
	// write declaration for first version
	if vnum.First() {
		if comm.nowrite {
			comm.logger.Info("skipping object declaration", "object_path", objPath)
		} else {
			decl := ocfl.Declaration{
				Type:    ocfl.DeclObject,
				Version: comm.newInv.Type.Spec,
			}
			if err := ocfl.WriteDeclaration(ctx, writeFS, objPath, decl); err != nil {
				return err
			}
		}
	}
	// transfer files
	srcFS, stageRoot := comm.stage.Root()
	for src, dst := range xfers {
		dst = path.Join(objPath, dst)
		xfers[src] = dst
		if comm.nowrite {
			comm.logger.Info("skipping file transfer", "src", src, "dst", dst)
		}
	}
	// fixme -- xfer keys are paths relative to stage root dir, not stage FS
	remap := make(map[string]string, len(xfers))
	for src, dst := range xfers {
		remap[path.Join(stageRoot, src)] = dst
	}
	xfers = remap
	if !comm.nowrite {
		_, err := xfer.DigestXfer(ctx, srcFS, writeFS, xfers)
		if err != nil {
			return fmt.Errorf("while transfering content files: %w", err)
		}
	}
	vPath := path.Join(objPath, vnum.String())
	if comm.nowrite {
		comm.logger.Info("skipping inventory write", "object_path", objPath, "version_path", vPath)
	} else {
		if err := WriteInventory(ctx, writeFS, comm.newInv, objPath, vPath); err != nil {
			return err
		}
	}
	return nil
}

func transferMap(newInv *Inventory, stageMan *digest.Map) (map[string]string, error) {
	xfer := map[string]string{}
	if newInv == nil || newInv.Manifest == nil {
		return nil, errors.New("stage is not complete")
	}
	for p, d := range newInv.Manifest.AllPaths() {
		if !strings.HasPrefix(p, newInv.Head.String()+"/") {
			continue
		}
		sources := stageMan.DigestPaths(d)
		if len(sources) == 0 {
			return nil, fmt.Errorf("no source file provided for digest: %s", d)
		}
		xfer[sources[0]] = p
	}
	return xfer, nil
}

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
func WithUser(name, addr string) CommitOption {
	return func(comm *commit) {
		comm.user.Name = name
		comm.user.Address = addr
	}
}

// WithNoWrite configures the commit to prevent any writes to the storage root.
// This enables "dry run" commits.
func WithNoWrite() CommitOption {
	return func(comm *commit) {
		comm.nowrite = true
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
