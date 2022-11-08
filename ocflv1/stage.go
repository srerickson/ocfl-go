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

// objectStage represents an OCFL object transformation
type objectStage struct {
	id      string      // object id from Commit()
	index   *ocfl.Index // index from Commit()
	prevInv *Inventory  // for

	// object options
	vnum            ocfl.VNum       // version number for the stage
	spec            ocfl.Spec       // OCFL spec for new version
	alg             digest.Alg      // digest alg
	contentDir      string          // content directory setting
	contentPathFunc ContentPathFunc // function used to configure content paths
	progress        io.Writer
	user            User
	message         string
	nowrite         bool    // used for "dry run" commit
	srcFS           ocfl.FS // source FS for paths in NewManifest
	logger          logr.Logger

	// generated
	state    *digest.Map // new version state: index.StateMap(alg)
	manifest *digest.Map // manifest relative to srFS index.ManifestMap
}

func newStage(id string, idx *ocfl.Index, prev *Inventory, opts ...ObjectOption) (*objectStage, error) {
	stg := &objectStage{
		id:              id,
		alg:             digest.SHA512(),
		spec:            defaultSpec,
		vnum:            ocfl.V(1),
		contentPathFunc: DefaultContentPathFunc,
		index:           idx,
		prevInv:         prev,
		srcFS:           idx.FS,
		logger:          logr.Discard(),
	}
	for _, opt := range opts {
		opt(stg)
	}
	if prev != nil {
		nextv, err := prev.Head.Next()
		if err != nil {
			return nil, fmt.Errorf("version scheme doesn't support versions after %s: %w", prev.Head, err)
		}
		stg.vnum = nextv                                // this ignore VNum options
		stg.contentDir = prev.ContentDirectory          // ignore ContentDirectory Option
		stg.alg, err = digest.Get(prev.DigestAlgorithm) // ignore DigestAlg option
		if err != nil {
			return nil, fmt.Errorf("cannot build stage from inventory: %w", err)
		}
	}
	manifest, err := idx.ManifestMap(stg.alg.ID())
	if err != nil {
		return nil, fmt.Errorf("cannot build manifest from index using %s: %w", stg.alg, err)
	}
	state, err := idx.StateMap(stg.alg.ID())
	if err != nil {
		return nil, fmt.Errorf("cannot build version state from index using %s: %w", stg.alg, err)
	}
	stg.manifest = manifest
	stg.state = state
	if prev != nil {
		if err := stg.validate(prev); err != nil {
			return nil, fmt.Errorf("stage options are not valid for this object: %w", err)
		}
	}
	return stg, nil
}

type ObjectOption func(*objectStage)

// WithAlg is used to set the digest algorithm for the first version of an
// object. It is ignored for subsequent versions.
func WithAlg(alg digest.Alg) ObjectOption {
	return func(stage *objectStage) {
		stage.alg = alg
	}
}

// WithContentDir is used to set the ContentDirectory value for the first
// version of an object. It is ignored for subsequent versions.
func WithContentDir(cd string) ObjectOption {
	return func(stage *objectStage) {
		stage.contentDir = cd
	}
}

// WithVersionPadding is used to set the version number padding for the first
// version of an object. It is ignored for subsequent versions.
func WithVersionPadding(p int) ObjectOption {
	return func(stage *objectStage) {
		stage.vnum = ocfl.V(stage.vnum.Num(), p)
	}
}

// WithContentPathFunc is a functional option used to set the stage's content path
// function
func WithContentPathFunc(fn ContentPathFunc) ObjectOption {
	return func(stage *objectStage) {
		stage.contentPathFunc = fn
	}
}

func WithMessage(msg string) ObjectOption {
	return func(stage *objectStage) {
		stage.message = msg
	}
}

func WithUser(name, addr string) ObjectOption {
	return func(stage *objectStage) {
		stage.user.Name = name
		stage.user.Address = addr
	}
}

// WithNoWrite configures the commit to prevent any writes to the storage root.
// This enables "dry run" commits.
func WithNoWrite() ObjectOption {
	return func(stage *objectStage) {
		stage.nowrite = true
	}
}

func WithProgressWriter(w io.Writer) ObjectOption {
	return func(stage *objectStage) {
		stage.progress = w
	}
}

func WithLogger(logger logr.Logger) ObjectOption {
	return func(stage *objectStage) {
		stage.logger = logger
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
func (stage *objectStage) nextManifest(prev *digest.Map) (*digest.Map, error) {
	var man *digest.Map
	if prev != nil {
		man = prev.Copy()
	} else {
		man = digest.NewMap()
	}
	if stage.contentPathFunc == nil {
		stage.contentPathFunc = DefaultContentPathFunc
	}
	if stage.contentDir == "" {
		stage.contentDir = contentDir
	}
	walkfn := func(p string, isdir bool, inf *ocfl.IndexItem) error {
		if isdir {
			return nil
		}
		sum, ok := inf.Digests[stage.alg.ID()]
		if !ok {
			return fmt.Errorf("missing digest for '%s'", stage.alg)
		}
		if man.DigestExists(sum) {
			return nil
		}
		// content path in manifest
		cont := stage.contentPathFunc(p, sum)
		cont = path.Join(stage.vnum.String(), stage.contentDir, cont)
		if err := man.Add(sum, cont); err != nil {
			return err
		}
		return nil
	}
	if err := stage.index.Walk(walkfn); err != nil {
		return nil, err
	}
	return man, nil
}

// nextInventory generates the next inventory based the previous inventory (if it exists)
func (stage *objectStage) nextInventory(prevInv *Inventory) (*Inventory, error) {
	var inv *Inventory
	var prevMan *digest.Map
	if prevInv != nil {
		if err := stage.validate(prevInv); err != nil {
			return nil, fmt.Errorf("the object settings are not compatible with the existing object: %w", err)
		}
		prevMan = prevInv.Manifest
	}
	newMan, err := stage.nextManifest(prevMan)
	if err != nil {
		return nil, fmt.Errorf("error while building manifest: %w", err)
	}
	if prevInv == nil {
		inv = &Inventory{
			ID:               stage.id,
			Head:             stage.vnum,
			Type:             stage.spec.AsInvType(),
			DigestAlgorithm:  stage.alg.ID(),
			ContentDirectory: stage.contentDir,
			Manifest:         newMan,
			Versions:         map[ocfl.VNum]*Version{},
		}
	} else {
		inv = prevInv.Copy()
		inv.Head = stage.vnum
		inv.Manifest = newMan
	}
	// add the new version directory to the Inventory object
	inv.Versions[inv.Head] = &Version{
		Created: time.Now().Truncate(time.Second),
		State:   stage.state,
		Message: stage.message,
	}
	// only add user if name from stage is not empty
	if stage.user.Name != "" {
		inv.Versions[inv.Head].User = &User{
			Name:    stage.user.Name,
			Address: stage.user.Address,
		}
	}
	if err := inv.Validate().Err(); err != nil {
		return nil, err
	}
	return inv, nil
}

// stageErrs checks that stage represent a valid next version for the inventory
func (stage *objectStage) validate(inv *Inventory) error {
	if inv.ID != stage.id {
		return fmt.Errorf("new version ID doesn't match previous ID: %s", stage.id)
	}
	next, err := inv.Head.Next()
	if err != nil {
		return fmt.Errorf("the version numbering scheme does support another version: %w", err)
	}
	if next != stage.vnum {
		return fmt.Errorf("new version number (%s) is not a valid successor of previous (%s)",
			stage.vnum, inv.Head)
	}
	if stage.alg.ID() != inv.DigestAlgorithm {
		return fmt.Errorf("new version must have same digest algorith as previous: %s", inv.DigestAlgorithm)
	}
	if inv.Type.Spec.Cmp(stage.spec) > 0 {
		return errors.New("new version cannot have lower OCFL spec than previous version")
	}
	cd := func(c string) string {
		if c == "" {
			return contentDir
		}
		return c
	}
	if cd(inv.ContentDirectory) != cd(stage.contentDir) {
		return errors.New("stage and inventory have different contentDirectory settings")
	}
	// all digests in stage index should be accounted for in either the
	// the stage's "add" manifest or the inventory manifest
	stateMap, err := stage.index.StateMap(stage.alg.ID())
	if err != nil {
		return fmt.Errorf("stage state is invalid: %w", err)
	}
	for digest := range stateMap.AllDigests() {
		if inv.Manifest.DigestExists(digest) {
			continue
		}
		if stage.manifest.DigestExists(digest) {
			continue
		}
		return fmt.Errorf("stage includes a digest with no known source: %s", digest)
	}
	return nil
}
