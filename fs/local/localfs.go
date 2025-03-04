package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

const (
	dirPerm  = 0755
	filePerm = 0644
)

type FS struct {
	ocflfs.ReadDirFS
	// path is os-specific path to a directory
	path string
}

var _ ocflfs.WriteFS = (*FS)(nil)
var _ ocflfs.ReadDirFS = (*FS)(nil)

func NewFS(path string) (*FS, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("new backend: %w", err)
	}
	return &FS{
		path:      abs,
		ReadDirFS: ocflfs.NewFS(os.DirFS(abs)),
	}, nil
}

func (fsys *FS) Root() string {
	return fsys.path
}

func (fsys *FS) Write(ctx context.Context, name string, src io.Reader) (int64, error) {
	fullPath, err := fsys.osPath(name)
	if err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	parent := filepath.Dir(fullPath)
	if err := os.MkdirAll(parent, dirPerm); err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	dst, err := os.OpenFile(fullPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, filePerm)
	if err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	n, err := io.Copy(dst, src)
	if err != nil {
		dst.Close()
		return n, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	if err := dst.Close(); err != nil {
		return n, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	return n, nil
}

func (fsys *FS) Remove(ctx context.Context, name string) error {
	fullPath, err := fsys.osPath(name)
	if err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	if name == "." {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  errors.New("cannot remove top-level directory"),
		}
	}
	if err := ctx.Err(); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	if err := os.Remove(fullPath); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	return nil
}

func (fsys *FS) RemoveAll(ctx context.Context, name string) error {
	fullPath, err := fsys.osPath(name)
	if err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	if name == "." {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  errors.New("cannot remove top-level directory"),
		}
	}
	if err := ctx.Err(); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	if err := os.RemoveAll(fullPath + "/"); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	return nil
}

func (fsys *FS) osPath(name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", fs.ErrInvalid
	}
	return filepath.Join(fsys.path, filepath.FromSlash(name)), nil
}
