package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	var inv *Inventory
	obj, err := s.GetObject(ctx, id)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("getting object info from storage root: %w", err)
	}
	if obj != nil {
		inv, err = obj.Inventory(ctx)
		if err != nil {
			return err
		}
	}
	comm, err := newCommit(id, stage, inv, opts...)
	if err != nil {
		return err
	}
	return s.commit(ctx, comm)
}

// commit creates or updates an object in the store using stage.
func (s *Store) commit(ctx context.Context, comm *commit) error {
	writeFS, objPath, err := s.objectWriteFSPath(comm.id)
	if err != nil {
		return err
	}
	// file transfer list
	xfers, err := transferMap(comm.newInv, comm.manifest)
	if err != nil {
		return err
	}
	if fsys, _ := comm.stage.Root(); len(xfers) > 0 && fsys == nil {
		return fmt.Errorf("stage doesn't provide an FS for reading files to upload")
	}
	comm.logger.Info("committing new object version", "id", comm.id, "head", comm.vnum, "alg", comm.alg, "message", comm.message)
	// expect version directory to ErrNotExist or be empty
	if comm.vnum.First() {
		entries, err := s.fsys.ReadDir(ctx, objPath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if len(entries) != 0 {
			return errors.New("object directory must be empty to commit")
		}
	} else {
		entries, err := s.fsys.ReadDir(ctx, path.Join(objPath, comm.vnum.String()))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("version directory '%s' must be empty to commit", comm.vnum.String())
		}
	}

	// write declaration for first version
	if comm.vnum.First() {
		if comm.nowrite {
			comm.logger.Info("skipping object declaration", "object_path", objPath)
		} else {
			decl := ocfl.Declaration{
				Type:    ocfl.DeclObject,
				Version: comm.spec,
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
		_, err := xfer.DigestXfer(ctx, srcFS, writeFS, xfers, xfer.WithProgress(comm.progress))
		if err != nil {
			return fmt.Errorf("while transfering content files: %w", err)
		}
	}
	vPath := path.Join(objPath, comm.vnum.String())
	if comm.nowrite {
		comm.logger.Info("skipping inventory write", "object_path", objPath, "version_path", vPath)
	} else {
		if err := WriteInventory(ctx, writeFS, comm.newInv, objPath, vPath); err != nil {
			return err
		}
	}
	return nil
}

// get the writeFS and object path for an object
func (s *Store) objectWriteFSPath(objID string) (ocfl.WriteFS, string, error) {
	writeFS, ok := s.fsys.(ocfl.WriteFS)
	if !ok {
		return nil, "", fmt.Errorf("storage root backend is read-only")
	}
	if s.layoutFunc == nil {
		return nil, "", fmt.Errorf("storage root layout must be set to commit: %w", ErrLayoutUndefined)
	}
	objPath, err := s.layoutFunc(objID)
	if err != nil {
		return nil, "", fmt.Errorf("object ID must be valid to commit: %w", err)
	}
	return writeFS, path.Join(s.rootDir, objPath), nil
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

// commit represents an OCFL object transformation
type commit struct {
	id      string // object id from Commit()
	stage   *ocfl.Stage
	prevInv *Inventory

	// object options
	vnum            ocfl.VNum       // version number for the stage
	spec            ocfl.Spec       // OCFL spec for new version
	contentDir      string          // content directory setting
	contentPathFunc ContentPathFunc // function used to configure content paths
	user            User
	message         string
	nowrite         bool // used for "dry run" commit
	logger          logr.Logger

	progress io.Writer
	// generated from stage
	alg      digest.Alg
	newInv   *Inventory
	manifest *digest.Map // stage manifest (i.e., paths relative to stage's FS)
}

func newCommit(id string, stage *ocfl.Stage, prev *Inventory, opts ...CommitOption) (*commit, error) {
	comm := &commit{
		id:              id,
		spec:            defaultSpec,
		vnum:            ocfl.V(1),
		contentPathFunc: DefaultContentPathFunc,
		stage:           stage,
		prevInv:         prev,
		alg:             stage.DigestAlg(),
		logger:          logr.Discard(),
	}
	for _, opt := range opts {
		opt(comm)
	}
	if prev != nil {
		nextv, err := prev.Head.Next()
		if err != nil {
			return nil, fmt.Errorf("version scheme doesn't support versions after %s: %w", prev.Head, err)
		}
		comm.vnum = nextv                       // ignoring any version number/padding options
		comm.contentDir = prev.ContentDirectory // ignoring content directory options
	}
	newInv, err := NewVersionInventory(stage, comm.prevInv, time.Now(), comm.message, &comm.user)
	if err != nil {
		return nil, err
	}

	newInv.ID = id
	comm.newInv = newInv
	comm.manifest, err = stage.Manifest(nil)
	if err != nil {
		return nil, err
	}
	if prev != nil {
		if err := comm.validate(prev); err != nil {
			return nil, fmt.Errorf("stage options are not valid for this object: %w", err)
		}
	}
	return comm, nil
}

type CommitOption func(*commit)

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
		comm.vnum = ocfl.V(comm.vnum.Num(), p)
	}
}

// WithContentPathFunc is a functional option used to set the stage's content path
// function.
func WithContentPathFunc(fn ContentPathFunc) CommitOption {
	return func(comm *commit) {
		comm.contentPathFunc = fn
	}
}

func WithMessage(msg string) CommitOption {
	return func(comm *commit) {
		comm.message = msg
	}
}

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

func WithProgressWriter(w io.Writer) CommitOption {
	return func(comm *commit) {
		comm.progress = w
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

// stageErrs checks that stage represent a valid next version for the inventory
func (comm *commit) validate(inv *Inventory) error {
	if inv.ID != comm.id {
		return fmt.Errorf("new version ID doesn't match previous ID: %s", comm.id)
	}
	next, err := inv.Head.Next()
	if err != nil {
		return fmt.Errorf("the version numbering scheme does support another version: %w", err)
	}
	if next != comm.vnum {
		return fmt.Errorf("new version number (%s) is not a valid successor of previous (%s)",
			comm.vnum, inv.Head)
	}
	if comm.alg.ID() != inv.DigestAlgorithm {
		return fmt.Errorf("new version must have same digest algorith as previous: %s", inv.DigestAlgorithm)
	}
	if inv.Type.Spec.Cmp(comm.spec) > 0 {
		return errors.New("new version cannot have lower OCFL spec than previous version")
	}
	cd := func(c string) string {
		if c == "" {
			return contentDir
		}
		return c
	}
	if cd(inv.ContentDirectory) != cd(comm.contentDir) {
		return errors.New("stage and inventory have different contentDirectory settings")
	}
	// all digests in stage index should be accounted for in either the
	// the stage's "add" manifest or the inventory manifest
	stateMap := comm.stage.VersionState()
	for _, digest := range stateMap.AllDigests() {
		if inv.Manifest.HasDigest(digest) {
			continue
		}
		if comm.manifest.HasDigest(digest) {
			continue
		}
		return fmt.Errorf("stage includes a digest with no known source: %s", digest)
	}
	return nil
}
