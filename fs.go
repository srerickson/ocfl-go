package ocfl

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/srerickson/ocfl-go/internal/walkdirs"
)

var ErrNotFile = errors.New("not a file")

// FS is a minimal, read-only storage layer abstraction. It is similar to the
// standard library's io/fs.FS, except it uses contexts and OpenFile is not
// required to gracefully handle directories.
type FS interface {
	OpenFile(ctx context.Context, name string) (fs.File, error)
	ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error)
}

// WriteFS is a storage backend that supports write and remove operations.
type WriteFS interface {
	FS
	Write(ctx context.Context, name string, buffer io.Reader) (int64, error)
	// Remove the file with path name
	Remove(ctx context.Context, name string) error
	// Remove the directory with path name and all its contents. If the path
	// does not exist, return nil.
	RemoveAll(ctx context.Context, name string) error
}

// CopyFS is a storage backend that supports copying files.
type CopyFS interface {
	WriteFS
	// Copy creates or updates the file at dst with the contents of src. If dst
	// exists, it should be overwritten
	Copy(ctx context.Context, dst string, src string) error
}

// Files walks the directory tree under root, calling fn
func Files(ctx context.Context, fsys FS, pth PathSelector, fn func(name string) error) error {
	if iter, ok := fsys.(FileIterator); ok {
		return iter.Files(ctx, pth, fn)
	}
	walkFn := func(dir string, entries []fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !e.Type().IsRegular() {
				// TODO: log symlinks and irregular files as warnings
				continue
			}
			if err := fn(path.Join(dir, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	return walkdirs.WalkDirs(ctx, fsys, pth.Path(), pth.SkipDir, walkFn, 0)
}

// FileIterator is used to iterate over regular files in an FS
type FileIterator interface {
	FS
	// Files calls a function for each filename satisfying the path selector.
	// The function should only be called for "regular" files (never for
	// directories or symlinks).
	Files(context.Context, PathSelector, func(name string) error) error
}

// PathSelector is used to configure iterators that walk an FS. See FileIterator and
// ObjectRootIterator.
type PathSelector struct {
	// Dir is a parent directory for all paths that satisfy the selector. All
	// paths in the selection match Dir or have Dir as a parent (prefix). If Dir
	// is not a well-formed path (see fs.ValidPath), then no path names will
	// satisfy the path selector. There is one exception: The empty string is
	// converted to "." by consumers of the selector using Path().
	Dir string

	// SkipDirFn is used to skip directories during an iteration process. If the
	// function returns true for a given path, the directory's contents will be
	// skipped. The string parameter is always a directory name relative to an
	// FS.
	SkipDirFn func(dir string) bool
}

// Dir is a convenient way to construct a PathSelector for a given directory.
func Dir(name string) PathSelector { return PathSelector{Dir: name} }

// Path returns name as a valid path or an empty string if name is not a
// valid path
func (ps PathSelector) Path() string {
	if ps.Dir == "" {
		return "."
	}
	if !fs.ValidPath(ps.Dir) {
		return ""
	}
	return ps.Dir
}

// SkipDir returns true if dir should be skipped during an interation process
// that uses the path selector
func (ps PathSelector) SkipDir(name string) bool {
	if !fs.ValidPath(name) {
		return true
	}
	d := ps.Dir
	if d == "." {
		d = ""
	}
	if !strings.HasPrefix(name, d) {
		return true
	}
	if ps.SkipDirFn != nil {
		return ps.SkipDirFn(name)
	}
	return false
}

type ioFS struct {
	fs.FS
}

// NewFS wraps an io/fs.FS as an ocfl.FS
func NewFS(fsys fs.FS) FS { return &ioFS{FS: fsys} }

// DirFS is shorthand for NewFS(os.DirFS(dir))
func DirFS(dir string) FS { return NewFS(os.DirFS(dir)) }

func (fsys *ioFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, &fs.PathError{
			Op:   "openfile",
			Path: name,
			Err:  err,
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
func (fsys *ioFS) ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, &fs.PathError{
			Op:   "readdir",
			Path: name,
			Err:  err,
		}
	}
	dirents, err := fs.ReadDir(fsys.FS, name)
	if err != nil {
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			// pathErr's Path will be system path;
			// change it to name
			pathErr.Path = name
		}
		return nil, err
	}
	return dirents, nil

}
