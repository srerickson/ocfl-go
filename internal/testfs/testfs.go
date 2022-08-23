package testfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/srerickson/ocfl"
)

const (
	dirPerm  = 0755
	filePerm = 0644
)

type TestFS struct {
	ocfl.FS
	// path is os-specific path to backend
	// base directory
	path string
}

var _ ocfl.WriteFS = (*TestFS)(nil)

func NewTestFS(path string) (*TestFS, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("new backend: %w", err)
	}
	return &TestFS{
		path: abs,
		FS:   ocfl.NewFS(os.DirFS(abs)),
	}, nil
}

func (fsys *TestFS) Root() string {
	return fsys.path
}

func (fsys *TestFS) Write(ctx context.Context, name string, src io.Reader) (int64, error) {
	if !fs.ValidPath(name) {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  errors.New("invalid path"),
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	fullPath := filepath.Join(fsys.path, filepath.FromSlash(name))
	parent := filepath.Dir(fullPath)
	err := os.MkdirAll(parent, dirPerm)
	if err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: fullPath,
			Err:  err,
		}
	}
	dst, err := os.OpenFile(fullPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, filePerm)
	if err != nil {
		return 0, err
	}
	defer dst.Close()
	return io.Copy(dst, src)
}
