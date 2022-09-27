package ocfl

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"

	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/digest/checksum"
)

// FS is a minimal, read-only storage layer abstraction. It is similar to the
// standard library's io/fs.FS, except it uses contexts and OpenFile is not
// required to gracefully handle directories.
type FS interface {
	OpenFile(ctx context.Context, name string) (fs.File, error)
	ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error)
}

// WriteFS is a storage layer abstraction that support write operations.
type WriteFS interface {
	FS
	Write(ctx context.Context, name string, buffer io.Reader) (int64, error)
}

// NewFS wraps an io/fs.FS as an ocfl.FS
func NewFS(fsys fs.FS) FS {
	return &ioFS{FS: fsys}
}

// DirFS is shorthand for NewFS(os.DirFS(dir))
func DirFS(dir string) FS {
	return NewFS(os.DirFS(dir))
}

type ioFS struct {
	fs.FS
}

func (fsys *ioFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, &fs.PathError{
			Op:   "readdir",
			Path: name,
			Err:  err,
		}
	}
	return fsys.Open(name)
}
func (fsys *ioFS) ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, &fs.PathError{
			Op:   "readdir",
			Path: name,
			Err:  err,
		}
	}
	return fs.ReadDir(fsys.FS, name)
}

// EachFile is a simple file walker. It's not very good and should be replaced: need better
// error handling when non-regular files are encountered.
func EachFile(ctx context.Context, fsys FS, root string, walkFn fs.WalkDirFunc) error {
	entries, err := fsys.ReadDir(ctx, root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		next := path.Join(root, e.Name())
		if e.Type().IsRegular() {
			err := walkFn(next, e, nil)
			if err != nil {
				return err
			}
		}
		if e.Type().IsDir() {
			err := EachFile(ctx, fsys, next, walkFn)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// DirTree build a digest.Tree from the contents of dir in fsys using algs. All
// paths in the tree are relative to fsys.
func DirTree(ctx context.Context, fsys FS, dir string, algs []digest.Alg, opts ...checksum.Option) (*digest.Tree, error) {
	tree := &digest.Tree{}
	setup := func(add checksum.AddFunc) error {
		walkfn := func(name string, e fs.DirEntry, err error) error {
			if err != nil {
				return fmt.Errorf("during source directory scan: %w", err)
			}
			if !add(name, algs) {
				return fmt.Errorf("source directory scan ended prematurely")
			}
			return nil
		}
		return EachFile(ctx, fsys, dir, walkfn)
	}
	cb := func(name string, result digest.Set, err error) error {
		if err != nil {
			return err
		}
		return tree.SetDigests(name, result, false)
	}
	open := func(name string) (io.Reader, error) {
		f, err := fsys.OpenFile(ctx, name)
		if err != nil {
			return nil, err
		}
		return f, nil
	}
	opts = append(opts, checksum.WithOpenFunc(open))
	if err := checksum.Run(setup, cb, opts...); err != nil {
		return nil, err
	}
	return tree, nil
}
