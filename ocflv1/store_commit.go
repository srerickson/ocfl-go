package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
)

func (s *Store) Commit(ctx context.Context, id string, index *ocfl.Index, opts ...ObjectOption) error {
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
	stage, err := newStage(id, index, inv, opts...)
	if err != nil {
		return err
	}
	return s.commit(ctx, stage)
}

// commit creates or updates an object in the store using stage.
func (s *Store) commit(ctx context.Context, stage *objectStage) error {
	writeFS, objPath, err := s.objectWriteFSPath(stage.id)
	if err != nil {
		return err
	}
	// build new inventory from previous
	newInv, err := stage.nextInventory(stage.prevInv)
	if err != nil {
		return fmt.Errorf("building new version inventory: %w", err)
	}
	// file transfer list
	xfer, err := transferMap(newInv, stage.manifest)
	if err != nil {
		return err
	}
	if len(xfer) > 0 && stage.srcFS == nil {
		return fmt.Errorf("stage doesn't provide an FS for reading files to upload")
	}
	stage.logger.Info("committing new object version", "id", stage.id, "head", stage.vnum, "alg", stage.alg, "message", stage.message)
	// expect version directory to ErrNotExist or be empty
	if stage.vnum.First() {
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

	// write declaration for first version
	if stage.vnum.First() {
		if stage.nowrite {
			stage.logger.Info("skipping object declaration", "object_path", objPath)
		} else {
			decl := ocfl.Declaration{
				Type:    ocfl.DeclObject,
				Version: stage.spec,
			}
			if err := ocfl.WriteDeclaration(ctx, writeFS, objPath, decl); err != nil {
				return err
			}
		}
	}
	// transfer files
	for src, dst := range xfer {
		dst := path.Join(objPath, dst)
		f, err := stage.srcFS.OpenFile(ctx, src)
		if err != nil {
			return err
		}
		reader := io.Reader(f)
		if stage.progress != nil {
			reader = io.TeeReader(f, stage.progress)
		}
		if stage.nowrite {
			stage.logger.Info("skipping file transfer", "src", src, "dst", dst)
		} else {
			if _, err := writeFS.Write(ctx, dst, reader); err != nil {
				f.Close()
				return err
			}
		}
		f.Close()
	}
	vPath := path.Join(objPath, stage.vnum.String())
	if stage.nowrite {
		stage.logger.Info("skipping inventory write", "object_path", objPath, "version_path", vPath)
	} else {
		if err := WriteInventory(ctx, writeFS, newInv, objPath, vPath); err != nil {
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
