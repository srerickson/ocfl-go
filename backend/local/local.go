package local

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/srerickson/ocfl/backend"
)

const (
	dirPerm  = 0755
	filePerm = 0644
)

type Backend struct {
	fs.FS
	// path is os-specific path to backend
	// base directory
	path string
}

var _ backend.Interface = (*Backend)(nil)

func NewBackend(path string) (*Backend, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("new backend: %w", err)
	}
	return &Backend{
		path: abs,
		FS:   os.DirFS(abs),
	}, nil
}

func (fsys *Backend) Root() string {
	return fsys.path
}

func (fsys *Backend) Write(name string, src io.Reader) (int64, error) {
	if !fs.ValidPath(name) {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  errors.New("invalid path"),
		}
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

func (fsys *Backend) Copy(dst, src string) error {
	reader, err := fsys.Open(src)
	if err != nil {
		return err
	}
	defer reader.Close()
	info, err := reader.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return &fs.PathError{
			Op:   "copy",
			Path: src,
			Err:  errors.New("source is not a regular file"),
		}
	}
	_, err = fsys.Write(dst, reader)
	return err
}

func (fsys *Backend) RemoveAll(name string) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{
			Op:   "remove_all",
			Path: name,
			Err:  errors.New("invalid path"),
		}
	}
	if name == "." {
		return &fs.PathError{
			Op:   "remove_all",
			Path: name,
			Err:  errors.New("cannot remove backend root"),
		}
	}
	fullPath := filepath.Join(fsys.path, filepath.FromSlash(name))
	return os.RemoveAll(fullPath)
}

// Renames src to dst: src should exist but dst should not.

func (fsys *Backend) Rename(src, dst string) error {
	if !fs.ValidPath(src) {
		return &fs.PathError{
			Op:   "rename",
			Path: src,
			Err:  errors.New("invalid path"),
		}
	}
	if !fs.ValidPath(dst) {
		return &fs.PathError{
			Op:   "rename",
			Path: dst,
			Err:  errors.New("invalid path"),
		}
	}
	if strings.HasPrefix(dst, src+"/") {
		return &fs.PathError{
			Op:   "rename",
			Path: dst,
			Err:  fmt.Errorf("cannot move %s to subdirectory of itself", src),
		}
	}

	fullSrc := filepath.Join(fsys.path, filepath.FromSlash(src))
	fullDst := filepath.Join(fsys.path, filepath.FromSlash(dst))

	_, err := os.Stat(fullDst)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("rename: %w", err)
	}
	if err == nil {
		return fmt.Errorf("rename: new name exists: %s", dst)
	}
	err = os.MkdirAll(filepath.Dir(fullDst), dirPerm)
	if err != nil {
		return err
	}
	return os.Rename(fullSrc, fullDst)
}
