package ocfl

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/checksum"
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

// DirTree build a digest.Tree from the contents of dir in fsys using
// digest algorithm alg.
func DirTree(ctx context.Context, fsys FS, dir string, alg digest.Alg) (*digest.Tree, error) {
	tree := &digest.Tree{}
	setup := func(add checksum.AddFunc) error {
		walkfn := func(name string, e fs.DirEntry, err error) error {
			if err != nil {
				return fmt.Errorf("during source directory scan: %w", err)
			}
			if !add(name, checksum.HashSet{alg.ID(): alg.New}) {
				return fmt.Errorf("source directory scan ended prematurely")
			}
			return nil
		}
		return EachFile(ctx, fsys, dir, walkfn)
	}
	cb := func(name string, result checksum.HashResult, err error) error {
		if err != nil {
			return err
		}
		// logical paths added to the stage stage should be relative to srcDir
		logical := strings.TrimPrefix(name, dir+"/")
		sum := hex.EncodeToString(result[alg.ID()])
		return tree.SetDigest(logical, sum, false)
	}
	open := func(name string) (io.ReadCloser, error) {
		return fsys.OpenFile(ctx, name)
	}
	if err := checksum.Run(setup, cb, checksum.WithOpenFunc(open)); err != nil {
		return nil, err
	}
	return tree, nil
}
