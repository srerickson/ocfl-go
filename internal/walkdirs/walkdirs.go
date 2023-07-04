package walkdirs

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"runtime"
)

type FS interface {
	ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error)
}

type SkipFunc func(string) bool

// ErrSkipDirs can be returned by a WalkDirsFunc to prevent WalkDirs from
// walking subdirectories.
var ErrSkipDirs = errors.New("skip subdirectories")

// WalkDirsFunc is a function called for each directory by WalkDirs. If
// the function returns ErrSkipDirs, none of the directory's subdirectories
// are walked.
type WalkDirsFunc func(name string, entries []fs.DirEntry, err error) error

// WalkDirs is a directory-oriented FS walker. It walks the FS starting at root,
// calling fn for each directory. If fn returns an error (other than
// ErrSkipDirs), the walk is canceled. WalkDirs reads directory entries in
// concurrent goroutines, the number of which is configurable. Each call to the
// WalkDirsFunc occurs from the same goroutine. The directory structure is
// walked depth-first order and in lexical order if concurrency is 1.
func WalkDirs(ctx context.Context, fsys FS, dir string, skipfn SkipFunc, fn WalkDirsFunc, gos int) error {
	if gos < 1 {
		gos = runtime.NumCPU()
	}
	readDirTask := func(dir string) ([]fs.DirEntry, error) {
		return fsys.ReadDir(ctx, dir)
	}
	var walkErr error
	// walkMgr is called for each directory and returns slice of paths to walk
	walkMgr := func(dir string, entries []fs.DirEntry, err error) ([]string, bool) {
		if fnErr := fn(dir, entries, err); fnErr != nil {
			if errors.Is(fnErr, ErrSkipDirs) {
				// don't add this directory's sub-directories
				return nil, true
			}
			walkErr = fnErr
			return nil, false
		}
		var subDirs []string // paths to continue search
		// evaluate entries in reverse order so they are in lexical order for
		// DoTailingTasks (which is LIFO). Note,
		for i := len(entries); i > 0; i-- {
			e := entries[i-1]
			if !e.IsDir() {
				continue
			}
			subdir := path.Join(dir, e.Name())
			if skipfn != nil && skipfn(subdir) {
				continue
			}
			subDirs = append(subDirs, subdir)
		}
		return subDirs, true
	}
	DoTailingTasks(gos, readDirTask, walkMgr, dir)
	return walkErr
}
