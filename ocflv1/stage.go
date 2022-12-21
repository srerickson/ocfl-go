package ocflv1

import (
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/go-logr/logr"
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
)

var ErrVersionExists = errors.New("version already exists")

// commit represents an OCFL object transformation
type commit struct {
	id      string // object id from Commit()
	stage   *ocfl.Stage
	prevInv *Inventory // for

	// object options
	vnum            ocfl.VNum       // version number for the stage
	spec            ocfl.Spec       // OCFL spec for new version
	contentDir      string          // content directory setting
	contentPathFunc ContentPathFunc // function used to configure content paths
	progress        io.Writer
	user            User
	message         string
	nowrite         bool // used for "dry run" commit
	logger          logr.Logger

	// generated from stage
	alg      digest.Alg
	state    *digest.Map // new version state
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
		comm.vnum = nextv                       // this ignore VNum options
		comm.contentDir = prev.ContentDirectory // ignore ContentDirectory Option
	}
	man, err := stage.Manifest()
	if err != nil {
		return nil, fmt.Errorf("stage is inconsistent: can't create manifest: %w", err)
	}
	comm.manifest = man
	comm.state = stage.VersionState()
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
// function
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

// nextManifest builds the next version of the manifest using stage stage
// and any previous manifest, which may be nil
func (comm *commit) nextManifest(prev *digest.Map) (*digest.Map, error) {
	var man *digest.Map
	if prev != nil {
		man = prev.Copy()
	} else {
		man = digest.NewMap()
	}
	if comm.contentPathFunc == nil {
		comm.contentPathFunc = DefaultContentPathFunc
	}
	if comm.contentDir == "" {
		comm.contentDir = contentDir
	}
	walkfn := func(p string, node *ocfl.Index) error {
		if node.IsDir() {
			return nil
		}
		sum, ok := node.Val().Digests[comm.alg.ID()]
		if !ok {
			return fmt.Errorf("missing digest for '%s'", comm.alg)
		}
		if man.DigestExists(sum) {
			return nil
		}
		// content path in manifest
		cont := comm.contentPathFunc(p, sum)
		cont = path.Join(comm.vnum.String(), comm.contentDir, cont)
		if err := man.Add(sum, cont); err != nil {
			return err
		}
		return nil
	}
	if err := comm.stage.Walk(walkfn); err != nil {
		return nil, err
	}
	return man, nil
}

// nextInventory generates the next inventory based the previous inventory (if it exists)
func (comm *commit) nextInventory(prevInv *Inventory) (*Inventory, error) {
	var inv *Inventory
	var prevMan *digest.Map
	if prevInv != nil {
		if err := comm.validate(prevInv); err != nil {
			return nil, fmt.Errorf("the object settings are not compatible with the existing object: %w", err)
		}
		prevMan = prevInv.Manifest
	}
	newMan, err := comm.nextManifest(prevMan)
	if err != nil {
		return nil, fmt.Errorf("error while building manifest: %w", err)
	}
	if prevInv == nil {
		inv = &Inventory{
			ID:               comm.id,
			Head:             comm.vnum,
			Type:             comm.spec.AsInvType(),
			DigestAlgorithm:  comm.alg.ID(),
			ContentDirectory: comm.contentDir,
			Manifest:         newMan,
			Versions:         map[ocfl.VNum]*Version{},
		}
	} else {
		inv = prevInv.Copy()
		inv.Head = comm.vnum
		inv.Manifest = newMan
	}
	// add the new version directory to the Inventory object
	inv.Versions[inv.Head] = &Version{
		Created: time.Now().Truncate(time.Second),
		State:   comm.state,
		Message: comm.message,
	}
	// only add user if name from stage is not empty
	if comm.user.Name != "" {
		inv.Versions[inv.Head].User = &User{
			Name:    comm.user.Name,
			Address: comm.user.Address,
		}
	}
	if err := inv.Validate().Err(); err != nil {
		return nil, err
	}
	return inv, nil
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
	for digest := range stateMap.AllDigests() {
		if inv.Manifest.DigestExists(digest) {
			continue
		}
		if comm.manifest.DigestExists(digest) {
			continue
		}
		return fmt.Errorf("stage includes a digest with no known source: %s", digest)
	}
	return nil
}
