package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/srerickson/ocfl"
)

var (
	ErrEmptyDirs     = errors.New("storage root includes empty directories")
	ErrNonObject     = errors.New("storage root includes files that aren't part of an object")
	ErrObjectVersion = errors.New("storage root includes objects with higher OCFL version than the storage root")
)

type ScanObjectsOpts struct {
	Strict      bool          // validate storage root structure
	Concurrency int           // numer of simultaneous readdir operations
	Timeout     time.Duration // timeout for readdir operations
}

// ScanObjects walks fsys from root returning a map of object root paths.
func ScanObjects(ctx context.Context, fsys ocfl.FS, root string, fn func(*Object) error, conf *ScanObjectsOpts) error {
	strict := false             // default: don't validate
	maxBatchLen := 4            // default: process up to 4 paths at a time
	timeout := time.Duration(0) // default: no timeout
	if conf != nil {
		strict = conf.Strict
		maxBatchLen = conf.Concurrency
		timeout = conf.Timeout
	}
	if maxBatchLen < 1 {
		maxBatchLen = 1
	}
	pathQ := []string{root}                  // queue of paths to scan
	extDir := path.Join(root, extensionsDir) // extensions path
	for {
		// process pathQ in batches, breaking if pathQ is empty
		batchLen := maxBatchLen
		qLen := len(pathQ)
		if qLen == 0 {
			break
		}
		if qLen < batchLen {
			batchLen = qLen
		}
		batch := make([]*storeScanJob, batchLen)
		batchWait := sync.WaitGroup{}
		for i := range batch {
			batch[i] = &storeScanJob{
				Path: pathQ[i],
			}
		}
		pathQ = pathQ[batchLen:]
		batchWait.Add(batchLen)
		for i := range batch {
			j := batch[i]
			go func() {
				jobCtx := ctx
				if timeout > 0 {
					var cancel context.CancelFunc
					jobCtx, cancel = context.WithTimeout(ctx, timeout)
					defer cancel()
				}
				j.Do(jobCtx, fsys)
				batchWait.Done()
			}()
		}
		batchWait.Wait()
		for _, result := range batch {
			if result.Err != nil {
				return result.Err
			}
			if result.Info.Declaration.Type == ocfl.DeclObject {
				obj := &Object{fsys: fsys, rootDir: result.Path, info: result.Info}
				if err := fn(obj); err != nil {
					return err
				}
				continue
			}
			if strict {
				switch result.Info.Declaration.Type {
				case ocfl.DeclStore:
					// store within a store is an error
					if result.Path != root {
						return fmt.Errorf("%w: %s", ErrNonObject, result.Path)
					}
				default:
					// directories without a declaration must include sub-directories
					// and only sub-directories -- however, the extensions directory
					// may be empty.
					if result.Empty() && result.Path != extDir {
						return fmt.Errorf("%w: %s", ErrEmptyDirs, result.Path)
					}
					if result.NumFiles > 0 {
						return fmt.Errorf("%w: %s", ErrNonObject, result.Path)
					}
				}
			}
			// don't continue scan into extensions sub-directories
			if result.Path == extDir {
				continue
			}
			// add sub-directories to scan queue
			pathQ = append(pathQ, result.Dirs...)
		}
	}
	return nil
}

// storeScanJob represents a readdir operation for store scanning
type storeScanJob struct {
	Path     string   // Path in the store to scan
	Err      error    // Errors from job
	Dirs     []string // sub directories
	Info     *ocfl.ObjInfo
	NumFiles int // number of regular files
}

func (j storeScanJob) Empty() bool {
	return len(j.Dirs) == 0 && j.NumFiles == 0
}

func (j *storeScanJob) Do(ctx context.Context, fsys ocfl.FS) {
	entries, err := fsys.ReadDir(ctx, j.Path)
	if err != nil {
		j.Err = err
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			j.Dirs = append(j.Dirs, path.Join(j.Path, e.Name()))
		} else if e.Type().IsRegular() {
			j.NumFiles++
		}
	}
	j.Info = ocfl.ObjInfoFromFS(entries)
}
