package fs

import (
	"context"
	"errors"
	"io/fs"
	"iter"
	"os"
)

// NewFS wraps an io/fs.FS as an FS
func NewFS(fsys fs.FS) ReadDirFS { return &ioFS{FS: fsys} }

// DirFS is shorthand for NewFS(os.DirFS(dir))
func DirFS(dir string) ReadDirFS { return NewFS(os.DirFS(dir)) }

// ioFS wraps and fs.FS
type ioFS struct {
	fs.FS
}

// OpenFile implementes FS for ioFS
func (fsys *ioFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, &fs.PathError{
			Op:   "openfile",
			Path: name,
			Err:  err,
		}
	}
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "openfile",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	f, err := fsys.Open(name)
	if err != nil {
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			// replace system path with name
			pathErr.Path = name
		}
		return nil, err
	}
	return f, nil
}

// ReadDir implements FS for ioFS.
func (fsys *ioFS) ReadDir(ctx context.Context, name string) iter.Seq2[fs.DirEntry, error] {
	return func(yield func(fs.DirEntry, error) bool) {
		if !fs.ValidPath(name) {
			yield(nil, &fs.PathError{
				Op:   "readdir",
				Path: name,
				Err:  fs.ErrInvalid,
			})
			return
		}
		entries, err := fs.ReadDir(fsys.FS, name)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				yield(nil, &fs.PathError{
					Op:   "readdir",
					Path: name,
					Err:  err,
				})
				return
			}
			if !yield(entry, nil) {
				return
			}
		}
		if err != nil {
			yield(nil, err)
		}
	}
}
