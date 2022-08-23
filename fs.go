package ocfl

import (
	"context"
	"io"
	"io/fs"
	"path"
)

type FS interface {
	OpenFile(ctx context.Context, name string) (fs.File, error)
	ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error)
}

type WriteFS interface {
	FS
	Write(ctx context.Context, name string, buffer io.Reader) (int64, error)
}

func NewFS(fsys fs.FS) FS {
	return &ioFS{FS: fsys}
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
