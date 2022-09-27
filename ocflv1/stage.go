package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/go-logr/logr"
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/digest/checksum"
)

var ErrVersionExists = errors.New("version already exists")

// Stage represents an OCFL object transformation
type Stage struct {
	id              string          // object id
	vnum            ocfl.VNum       // version number for the stage
	spec            ocfl.Spec       // OCFL spec for new version
	alg             digest.Alg      // digest alg
	state           *digest.Tree    // logical state for new version
	manifest        *digest.Map     // content to be added during commit
	srcFS           ocfl.FS         // source FS for paths in NewManifest
	contentDir      string          // content directory setting
	contentPathFunc ContentPathFunc // function used to configure content paths
}

type StageOpt func(*Stage)

// StageFS is functional option used to set the source FS for files
// added to the stage.
func StageFS(fsys ocfl.FS) StageOpt {
	return func(stage *Stage) {
		stage.srcFS = fsys
	}
}

// StageDigest
func StageDigestAlgorithm(alg digest.Alg) StageOpt {
	return func(stage *Stage) {
		stage.alg = alg
	}
}

// // StageContentDir is functional option used to set the stage's content directory
// func StageContentDir(cd string) StageOpt {
// 	return func(stage *Stage) {
// 		stage.contentDir = cd
// 	}
// }

// StageContentPathFunc is a functional option used to set the stage's content path
// function
func StageContentPathFunc(fn ContentPathFunc) StageOpt {
	return func(stage *Stage) {
		stage.contentPathFunc = fn
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

type StageAddConf struct {
	log      logr.Logger
	progress io.Writer
	dstDir   string
}

type StageAddOpt func(*StageAddConf)

func StageAddProgress(w io.Writer) StageAddOpt {
	return func(c *StageAddConf) {
		c.progress = w
	}
}

func (stage *Stage) AddDir(ctx context.Context, srcDir string, opts ...StageAddOpt) error {
	if stage.srcFS == nil {
		return fmt.Errorf("stage's source FS for added files is not set")
	}
	conf := StageAddConf{
		log:    logr.Discard(),
		dstDir: ".",
	}
	for _, o := range opts {
		o(&conf)
	}
	// digest map of srcdir
	// convert map to tree and and add tree to stage fs at logical dr
	tree, err := ocfl.DirTree(ctx, stage.srcFS, srcDir, []digest.Alg{stage.alg}, checksum.WithProgress(conf.progress))
	if err != nil {
		return fmt.Errorf("while scanning %s: %w", srcDir, err)
	}
	walkFn := func(n string, isdir bool, sums digest.Set) error {
		if isdir {
			return nil
		}
		sum, ok := sums[stage.alg]
		if !ok {
			return fmt.Errorf("missing required digest for '%s'", stage.alg)
		}
		return stage.addManifest(sum, n)
	}
	if err = tree.Walk(walkFn); err != nil {
		return err
	}
	sub, err := tree.Sub(srcDir)
	if err != nil {
		return fmt.Errorf("shouldn't happen, need better message: %w", err)
	}
	return stage.state.SetDir(conf.dstDir, sub, true)
}

func (stage *Stage) addManifest(sum, path string) error {
	if stage.manifest == nil {
		stage.manifest = digest.NewMap()
	}
	if stage.manifest.DigestExists(sum) {
		return nil
	}
	if err := stage.manifest.Add(sum, path); err != nil {
		return fmt.Errorf("adding digest to stage manifest: %w", err)
	}
	return nil
}

// buildManifestNext generates the manifest for the next version inventory based on
// the stage
func buildManifestNext(stage *Stage, prev *digest.Map) (*digest.Map, error) {
	var m *digest.Map
	if prev != nil {
		m = prev.Copy()
	} else {
		m = digest.NewMap()
	}
	if stage.contentPathFunc == nil {
		stage.contentPathFunc = DefaultContentPathFunc
	}
	if stage.contentDir == "" {
		stage.contentDir = contentDir
	}
	walkfn := func(p string, isdir bool, sums digest.Set) error {
		if isdir {
			return nil
		}
		digest, ok := sums[stage.alg]
		if !ok {
			return fmt.Errorf("missing digest for '%s'", stage.alg)
		}
		if m.DigestExists(digest) {
			return nil
		}
		// content path in manifest
		cont := stage.contentPathFunc(p, digest)
		cont = path.Join(stage.vnum.String(), stage.contentDir, cont)
		if err := m.Add(digest, cont); err != nil {
			return err
		}
		return nil
	}
	if err := stage.state.Walk(walkfn); err != nil {
		return nil, err
	}
	return m, nil
}

func buildInventoryV1(ctx context.Context, stage *Stage) (*Inventory, error) {
	manifest, err := buildManifestNext(stage, nil)
	if err != nil {
		return nil, fmt.Errorf("error while building manifest: %w", err)
	}
	state, err := stage.state.AsMap(stage.alg)
	if err != nil {
		return nil, fmt.Errorf("error while converting state: %w", err)
	}
	inv := &Inventory{
		ID:              stage.id,
		Head:            stage.vnum,
		Type:            stage.spec.AsInvType(),
		DigestAlgorithm: stage.alg,
		Manifest:        manifest,
		Versions:        map[ocfl.VNum]*Version{},
	}
	inv.Versions[inv.Head] = &Version{
		Created: time.Now().Truncate(time.Second),
		State:   state,
	}
	if err := inv.Validate().Err(); err != nil {
		return nil, err
	}
	return inv, nil
}

// buildInventoryNext creates a new inventory from a stage and a previous inventory.
func buildInventoryNext(ctx context.Context, stage *Stage, prev *Inventory) (*Inventory, error) {
	if err := stage.validate(prev); err != nil {
		return nil, fmt.Errorf("the stage is incompatible with the object inventory: %w", err)
	}
	inv := prev.Copy()
	inv.Head, _ = inv.Head.Next()
	var err error
	inv.Manifest, err = buildManifestNext(stage, inv.Manifest)
	if err != nil {
		return nil, fmt.Errorf("error while updating manifest: %w", err)
	}
	state, err := stage.state.AsMap(stage.alg)
	if err != nil {
		return nil, fmt.Errorf("error while converting state: %w", err)
	}
	// add the new version directory to the Inventory object
	inv.Versions[inv.Head] = &Version{
		Created: time.Now().Truncate(time.Second),
		State:   state,
	}
	if err := inv.Validate().Err(); err != nil {
		return nil, err
	}
	return inv, nil
}

// stageErrs checks that stage represent a valid next version for the inventory
func (stage *Stage) validate(inv *Inventory) error {
	if inv.ID != stage.id {
		return fmt.Errorf("stage ID doesn't match inventory ID: %s", stage.id)
	}
	next, err := inv.Head.Next()
	if err != nil {
		return fmt.Errorf("the version numbering scheme does support additional versions: %w", err)
	}
	if next != stage.vnum {
		return fmt.Errorf("stage version (%s) is not next for the inventory (%s)",
			stage.vnum, next)
	}
	if stage.alg != inv.DigestAlgorithm {
		return errors.New("stage and inventory have different digest algorithms")
	}
	if inv.Type.Spec.Cmp(stage.spec) > 0 {
		return errors.New("stage has lower OCFL spec version than inventory")
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
	// all digests in stage state should be accounted for in either the
	// the stage's "add" manifest or the inventory manifest
	stateMap, err := stage.state.AsMap(stage.alg)
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
