package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
)

// StageNew creates a stage for creating the first version on an object with the
// given id.
func (s *Store) StageNew(ctx context.Context, id string, opts ...StageOpt) (*Stage, error) {
	stage := &Stage{
		id:              id,
		vnum:            ocfl.V1,
		spec:            s.spec,
		alg:             digest.SHA512,
		contentDir:      contentDir,
		contentPathFunc: DefaultContentPathFunc,
		srcFS:           ocfl.NewFS(os.DirFS(".")),
		state:           &digest.Tree{},
		manifest:        digest.NewMap(),
	}
	for _, opt := range opts {
		opt(stage)
	}
	return stage, nil
}

// StageNext creates a stage for next the version of an existing object.
func (s *Store) StageNext(ctx context.Context, obj *Object, opts ...StageOpt) (*Stage, error) {
	inv, err := obj.Inventory(ctx)
	if err != nil {
		return nil, err
	}
	nextV, err := inv.Head.Next()
	if err != nil {
		return nil, err
	}
	state := &digest.Tree{}
	for p, d := range inv.Versions[inv.Head].State.AllPaths() {
		if err := state.SetDigest(p, inv.DigestAlgorithm, d, false); err != nil {
			return nil, err
		}
	}
	stage := &Stage{
		id:              inv.ID,
		vnum:            nextV,
		spec:            s.spec, // ocfl version from store
		alg:             inv.DigestAlgorithm,
		contentDir:      inv.ContentDirectory,
		contentPathFunc: DefaultContentPathFunc,
		state:           state,
		manifest:        digest.NewMap(),
	}
	for _, opt := range opts {
		opt(stage)
	}
	if err := stage.validate(inv); err != nil {
		return nil, fmt.Errorf("stage options are not valid for this object: %w", err)
	}
	return stage, nil
}

type CommitConf struct {
	progress io.Writer
}

type CommitOption func(*CommitConf)

func CommitProgress(w io.Writer) CommitOption {
	return func(c *CommitConf) {
		c.progress = w
	}
}

// Commit creates or updates an object in the store using stage.
func (s *Store) Commit(ctx context.Context, stage *Stage, opts ...CommitOption) error {
	writeFS, ok := s.fsys.(ocfl.WriteFS)
	if !ok {
		return fmt.Errorf("storage root backend is read-only")
	}
	var conf CommitConf
	for _, opt := range opts {
		opt(&conf)
	}
	var inv *Inventory
	var invErr error
	if stage.vnum.Num() == 1 {
		inv, invErr = buildInventoryV1(ctx, stage)
	} else {
		obj, err := s.GetObject(ctx, stage.id)
		if err != nil {
			return fmt.Errorf("retrieving object: %w", err)
		}
		prev, err := obj.Inventory(ctx)
		if err != nil {
			return fmt.Errorf("retrieving object inventory: %w", err)
		}
		inv, invErr = buildInventoryNext(ctx, stage, prev)
	}
	if invErr != nil {
		return fmt.Errorf("building inventory from stage: %w", invErr)
	}
	// safe to commit?
	if s.layoutFunc == nil {
		return fmt.Errorf("storage root layout must be set to commit: %w", ErrLayoutUndefined)
	}
	objPath, err := s.layoutFunc(stage.id)
	if err != nil {
		return fmt.Errorf("object ID must be valid to commit: %w", err)
	}
	objPath = path.Join(s.rootDir, objPath)
	// expect version directory to ErrNotExist or be empty
	if stage.vnum.Num() == 1 {
		entries, err := s.fsys.ReadDir(ctx, objPath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if len(entries) != 0 {
			return errors.New("object directory must be empty to commit")
		}
	} else {
		entries, err := s.fsys.ReadDir(ctx, path.Join(objPath, stage.vnum.String()))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("version directory '%s' must be empty to commit", stage.vnum.String())
		}
	}
	// copy files from srcFS to storage root fs
	for p, d := range inv.Manifest.AllPaths() {
		if !strings.HasPrefix(p, stage.vnum.String()) {
			continue
		}
		dst := path.Join(objPath, p)
		sources := stage.manifest.DigestPaths(d)
		if len(sources) == 0 {
			return fmt.Errorf("stage doesn't provide a source for digest: %s", d)
		}
		f, err := stage.srcFS.OpenFile(ctx, sources[0])
		if err != nil {
			return err
		}
		defer f.Close()
		reader := io.Reader(f)
		if conf.progress != nil {
			reader = io.TeeReader(f, conf.progress)
		}
		_, err = writeFS.Write(ctx, dst, reader)
		if err != nil {
			return err
		}
	}
	// write declaration and inventory
	decl := ocfl.Declaration{
		Type:    ocfl.DeclObject,
		Version: stage.spec,
	}
	if stage.vnum.Num() == 1 {
		if err := ocfl.WriteDeclaration(ctx, writeFS, objPath, decl); err != nil {
			return err
		}
	}
	if err := WriteInventory(ctx, writeFS, objPath, inv); err != nil {
		return err
	}
	if err := WriteInventory(ctx, writeFS, path.Join(objPath, stage.vnum.String()), inv); err != nil {
		return err
	}
	return nil
}
