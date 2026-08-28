package fs

import (
	"context"
	"errors"
	"io/fs"
	"iter"
	"os"
)

// NewWrapFS returns a *WrapFS for accessing files in fsys.
func NewWrapFS(fsys fs.FS) *WrapFS { return &WrapFS{FS: fsys} }

// DirFS is shorthand for NewFS(os.DirFS(dir))
func DirFS(dir string) *WrapFS { return NewWrapFS(os.DirFS(dir)) }

// WrapFS wraps an [io/fs.FS] and implements [DirEntriesFS].
//
// DirEntries follows [DirEntriesFS]: a listing that fails partway yields the
// entries it read and then the error, since fs.ReadDir(fsys.FS, name) already
// carries that shape from the wrapped FS.
type WrapFS struct {
	fs.FS
}

// OpenFile implementes FS for WrapFS
func (fsys *WrapFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
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

// DirEntries implements DirEntriesFS for WrapFS.
func (fsys *WrapFS) DirEntries(ctx context.Context, name string) iter.Seq2[fs.DirEntry, error] {
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
			if ctxErr := ctx.Err(); ctxErr != nil {
				// A pending ReadDir error must not be dropped just because
				// cancellation is noticed first: join them so a caller
				// matching either context.Canceled or the read failure with
				// errors.Is still finds it.
				cause := ctxErr
				if err != nil {
					cause = errors.Join(ctxErr, err)
				}
				yield(nil, &fs.PathError{
					Op:   "readdir",
					Path: name,
					Err:  cause,
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
