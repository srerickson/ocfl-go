package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl"
	"gocloud.dev/blob"
)

var ErrNotDir = fmt.Errorf("not a directory")

// FS is a generic backend for cloud storage based on gocloud.dev/blob
type FS struct {
	buck *blob.Bucket
}

var _ ocfl.WriteFS = (*FS)(nil)

func NewFS(b *blob.Bucket) *FS {
	return &FS{
		buck: b,
	}
}

func (b *FS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "openfile",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	reader, err := b.buck.NewReader(ctx, name, nil)
	if err != nil {
		return nil, &fs.PathError{
			Op:   "openfile",
			Path: name,
			Err:  err,
		}
	}
	return &file{
		ReadCloser: reader,
		info: &fileInfo{
			name:    path.Base(name),
			size:    reader.Size(),
			modTime: reader.ModTime(),
		},
	}, nil
}

func (b *FS) ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "readdir",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	const pageSize = 1000
	var (
		opts = &blob.ListOptions{
			Delimiter: "/",
		}
		token   = blob.FirstPageToken
		list    []*blob.ListObject
		err     error
		results []fs.DirEntry
	)
	if name != "." {
		opts.Prefix = name + "/"
	}
	for {
		list, token, err = b.buck.ListPage(ctx, token, pageSize, opts)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, &fs.PathError{
				Op:   "readdir",
				Path: name,
				Err:  err,
			}
		}
		for _, item := range list {
			inf := &fileInfo{
				name:    path.Base(item.Key),
				size:    item.Size,
				modTime: item.ModTime,
			}
			if item.IsDir {
				inf.mode = fs.ModeDir
			}
			results = append(results, inf)
		}
		if len(token) == 0 {
			break
		}
	}
	// if results are empty, the directory is considered non-existent (an
	// error), except when reading top-level directory
	if len(results) == 0 && name != "." {
		return nil, &fs.PathError{
			Op:   "readdir",
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}
	return results, nil
}

func (b *FS) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	opts := &blob.WriterOptions{}
	writer, err := b.buck.NewWriter(ctx, name, opts)
	if err != nil {
		return 0, err
	}
	n, writeErr := writer.ReadFrom(r)
	closeErr := writer.Close()
	if writeErr != nil {
		return n, writeErr
	}
	return n, closeErr
}
