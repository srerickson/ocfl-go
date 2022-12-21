package ocfl

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"time"
)

var ErrNotFile = errors.New("not a file")

// FS is a minimal, read-only storage layer abstraction. It is similar to the
// standard library's io/fs.FS, except it uses contexts and OpenFile is not
// required to gracefully handle directories.
type FS interface {
	OpenFile(ctx context.Context, name string) (fs.File, error)
	ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error)
}

// WriteFS is a storage layer abstraction that support write/remove operations.
type WriteFS interface {
	FS
	Write(ctx context.Context, name string, buffer io.Reader) (int64, error)
	// Remove the file with path name
	Remove(ctx context.Context, name string) error
	// Remove the directory with path name and all its contents. If the path
	// does not exist, return nil.
	RemoveAll(ctx context.Context, name string) error
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
			Op:   "openfile",
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

// EachFile walks the directory tree under root, calling walkFN on each file
// (not for directories). If a non-regulard file is found, walkFN is called with
// a non-nil error.
func EachFile(ctx context.Context, fsys FS, root string, walkFn fs.WalkDirFunc) error {
	fn := func(name string, d fs.DirEntry, err error) error {
		if d.Type().IsDir() {
			return err
		}
		if !d.Type().IsRegular() && err == nil {
			err = ErrNotFile
		}
		return walkFn(name, d, err)
	}
	return walk(ctx, fsys, root, fn)
}

// walk is similar to the standard library's io/fs.walk except that it takes
// a context and walkDirFn is not called for the top-level directory itself.
func walk(ctx context.Context, fsys FS, name string, walkDirFn fs.WalkDirFunc) error {
	err := walkDir(ctx, fsys, name, fakeDirEntry{name}, walkDirFn)
	if err == fs.SkipDir {
		return nil
	}
	return err
}

func walkDir(ctx context.Context, fsys FS, name string, d fs.DirEntry, walkDirFn fs.WalkDirFunc) error {
	// this code is adapated from the standard library's io/fs.Walk
	// Copyright 2020 The Go Authors. All rights reserved.
	// Use of this source code is governed by a BSD-style
	// license that can be found in Go's LICENSE file
	// https: //github.com/golang/go/blob/master/src/io/fs/walk.go
	if err := walkDirFn(name, d, ctx.Err()); err != nil || !d.IsDir() {
		if err == fs.SkipDir && d.IsDir() {
			// Successfully skipped directory.
			err = nil
		}
		return err
	}
	dirs, err := fsys.ReadDir(ctx, name)
	if err != nil {
		// Second call, to report ReadDir error.
		err = walkDirFn(name, d, err)
		if err != nil {
			if err == fs.SkipDir && d.IsDir() {
				err = nil
			}
			return err
		}
	}

	for _, d1 := range dirs {
		name1 := path.Join(name, d1.Name())
		if err := walkDir(ctx, fsys, name1, d1, walkDirFn); err != nil {
			if err == fs.SkipDir {
				break
			}
			return err
		}
	}
	return nil
}

// fakeDirEntry is used to fake dirinfo for the first call to walkDirInfo
type fakeDirEntry struct {
	name string
}

func (fake fakeDirEntry) Name() string               { return fake.name }
func (fake fakeDirEntry) IsDir() bool                { return true }
func (fake fakeDirEntry) Type() fs.FileMode          { return fs.ModeDir }
func (fake fakeDirEntry) Info() (fs.FileInfo, error) { return fake, nil }
func (fake fakeDirEntry) ModTime() time.Time         { return time.Time{} }
func (fake fakeDirEntry) Mode() fs.FileMode          { return 0777 | fs.ModeDir }
func (fake fakeDirEntry) Size() int64                { return 0 }
func (fake fakeDirEntry) Sys() any                   { return nil }

var _ fs.DirEntry = (*fakeDirEntry)(nil)
var _ fs.FileInfo = (*fakeDirEntry)(nil)
