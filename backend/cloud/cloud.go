package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/go-logr/logr"
	"github.com/srerickson/ocfl"
	"gocloud.dev/blob"
)

var ErrNotDir = fmt.Errorf("not a directory")

// FS is a generic backend for cloud storage backends using a blob.Bucket
type FS struct {
	*blob.Bucket
	log        logr.Logger
	writerOpts *blob.WriterOptions
	readerOpts *blob.ReaderOptions
}

var _ ocfl.WriteFS = (*FS)(nil)

type fsOption func(*FS)

func NewFS(b *blob.Bucket, opts ...fsOption) *FS {
	fsys := &FS{
		Bucket: b,
		log:    logr.Discard(),
	}
	for _, opt := range opts {
		opt(fsys)
	}
	return fsys
}

func WithLogger(l logr.Logger) fsOption {
	return func(fsys *FS) {
		fsys.log = l
	}
}

func (fsys *FS) WriterOptions(opts *blob.WriterOptions) *FS {
	return &FS{
		Bucket:     fsys.Bucket,
		log:        fsys.log,
		writerOpts: opts,
	}
}

func (fsys *FS) ReaderOptions(opts *blob.ReaderOptions) *FS {
	return &FS{
		Bucket:     fsys.Bucket,
		log:        fsys.log,
		readerOpts: opts,
	}
}

func (fsys *FS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	fsys.log.V(ocfl.LevelDebug).Info("open file", "name", name)
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "openfile",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	reader, err := fsys.Bucket.NewReader(ctx, name, fsys.readerOpts)
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

func (fsys *FS) ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error) {
	fsys.log.V(ocfl.LevelDebug).Info("read dir", "name", name)
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
		list, token, err = fsys.Bucket.ListPage(ctx, token, pageSize, opts)
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

func (fsys *FS) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	fsys.log.V(ocfl.LevelDebug).Info("write file", "name", name)
	writer, err := fsys.Bucket.NewWriter(ctx, name, fsys.writerOpts)
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
