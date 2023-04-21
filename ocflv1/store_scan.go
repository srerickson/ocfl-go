package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/carlmjohnson/workgroup"
	"github.com/srerickson/ocfl"
)

var (
	ErrEmptyDirs     = errors.New("storage root includes empty directories")
	ErrNonObject     = errors.New("storage root includes files that aren't part of an object")
	ErrObjectVersion = errors.New("storage root includes objects with higher OCFL version than the storage root")
)

type ScanObjectsOpts struct {
	Strict      bool // validate storage root structure
	Concurrency int  // numer of simultaneous readdir operations
}

func ScanObjects(ctx context.Context, fsys ocfl.FS, root string, fn func(*Object) error, conf *ScanObjectsOpts) error {
	strict := false                          // default: don't validate
	extDir := path.Join(root, extensionsDir) // extensions path
	numworkers := 4                          // default: number concurrent readdir workers
	if conf != nil {
		strict = conf.Strict
		numworkers = conf.Concurrency
	}
	readDirTask := func(dir string) ([]fs.DirEntry, error) {
		return fsys.ReadDir(ctx, dir)
	}
	scanMgr := func(dir string, entries []fs.DirEntry, err error) ([]string, error) {
		if err != nil {
			return nil, err
		}
		numfiles := 0
		var subDirs []string
		for _, e := range entries {
			if e.IsDir() {
				subDirs = append(subDirs, path.Join(dir, e.Name()))
			} else if e.Type().IsRegular() {
				numfiles++
			}
		}
		decl, _ := ocfl.FindDeclaration(entries)
		switch decl.Type {
		case ocfl.DeclObject:
			objRoot := ocfl.NewObjectRoot(fsys, dir, entries)
			if err := fn(&Object{ObjectRoot: *objRoot}); err != nil {
				return nil, err
			}
			return nil, nil // don't continue scan further into the object
		case ocfl.DeclStore:
			// store within a store is an error
			if strict && dir != root {
				return nil, fmt.Errorf("%w: %s", ErrNonObject, dir)
			}
		default:
			// directories without a declaration must include sub-directories
			// and only sub-directories -- however, the extensions directory
			// may be empty.
			if strict {
				if len(entries) == 0 && dir != extDir {
					return nil, fmt.Errorf("%w: %s", ErrEmptyDirs, dir)
				}
				if numfiles > 0 {
					return nil, fmt.Errorf("%w: %s", ErrNonObject, dir)
				}
			}
		}
		// don't continue scan into extensions sub-directories
		if dir == extDir {
			return nil, nil
		}
		return subDirs, nil
	}
	return workgroup.Do(numworkers, readDirTask, scanMgr, root)
}
