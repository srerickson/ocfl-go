// Package local implements an OCFL filesystem backend for a directory on the
// local filesystem. Each operation lives in its own file: write.go holds
// FS.Write and its atomic-write machinery, remove.go holds FS.Remove and
// FS.RemoveAll, and rename_posix.go / rename_windows.go hold the
// platform-specific rename-replace primitive Write relies on. This file holds
// the FS type itself and the path handling shared by all of them.
package local

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

type FS struct {
	ocflfs.DirEntriesFS
	// path is os-specific path to a directory
	path string
}

var _ ocflfs.WriteFS = (*FS)(nil)
var _ ocflfs.DirEntriesFS = (*FS)(nil)
var _ ocflfs.SameBackend = (*FS)(nil)

func NewFS(path string) (*FS, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("new backend: %w", err)
	}
	return &FS{
		path:         abs,
		DirEntriesFS: ocflfs.NewWrapFS(os.DirFS(abs)),
	}, nil
}

func (fsys *FS) Root() string {
	return fsys.path
}

// SameBackend implements [ocflfs.SameBackend]: it returns true only if other is
// also a *FS and its root path resolves to the same directory as fsys's root.
// Both root paths are absolutized with filepath.Abs (which also cleans them)
// before comparing, so trailing slashes, ".", "..", and relative paths don't
// cause false negatives. If the paths cannot be resolved, it returns false.
func (fsys *FS) SameBackend(other ocflfs.FS) bool {
	otherLocal, ok := other.(*FS)
	if !ok {
		return false
	}
	thisRoot, err := filepath.Abs(fsys.path)
	if err != nil {
		return false
	}
	otherRoot, err := filepath.Abs(otherLocal.path)
	if err != nil {
		return false
	}
	return thisRoot == otherRoot
}

func (fsys *FS) osPath(name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", fs.ErrInvalid
	}
	return filepath.Join(fsys.path, filepath.FromSlash(name)), nil
}
