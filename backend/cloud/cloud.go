package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/logging"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
	"golang.org/x/exp/slog"
)

var ErrNotDir = fmt.Errorf("not a directory")

// FS is a generic backend for cloud storage backends using a blob.Bucket
type FS struct {
	*blob.Bucket
	log        *slog.Logger
	writerOpts *blob.WriterOptions
	readerOpts *blob.ReaderOptions
}

var _ ocfl.WriteFS = (*FS)(nil)

type fsOption func(*FS)

func NewFS(b *blob.Bucket, opts ...fsOption) *FS {
	fsys := &FS{
		Bucket: b,
		log:    logging.DefaultLogger(),
	}
	for _, opt := range opts {
		opt(fsys)
	}
	return fsys
}

func WithLogger(l *slog.Logger) fsOption {
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
	fsys.log.DebugCtx(ctx, "open file", "name", name)
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "openfile",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	reader, err := fsys.Bucket.NewReader(ctx, name, fsys.readerOpts)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			err = errors.Join(err, fs.ErrNotExist)
		}
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
	fsys.log.DebugCtx(ctx, "read dir", "name", name)
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
			if gcerrors.Code(err) == gcerrors.NotFound {
				err = errors.Join(err, fs.ErrNotExist)
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
	fsys.log.DebugCtx(ctx, "write file", "name", name)
	if !fs.ValidPath(name) || name == "." {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	writer, err := fsys.Bucket.NewWriter(ctx, name, fsys.writerOpts)
	if err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	n, writeErr := writer.ReadFrom(r)
	closeErr := writer.Close()
	if writeErr != nil {
		return n, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  writeErr,
		}
	}
	if closeErr != nil {
		return n, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  closeErr,
		}
	}
	return n, nil
}

func (fsys *FS) Remove(ctx context.Context, name string) error {
	fsys.log.DebugCtx(ctx, "remove file", "name", name)
	if !fs.ValidPath(name) {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	if name == "." {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  errors.New("cannot remove top-level directory"),
		}
	}
	if err := fsys.Bucket.Delete(ctx, name); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}
	return nil
}

func (fsys *FS) RemoveAll(ctx context.Context, name string) error {
	fsys.log.DebugCtx(ctx, "remove dir", "name", name)
	if !fs.ValidPath(name) {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	if name == "." {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  errors.New("cannot remove top-level directory"),
		}
	}
	listOpt := &blob.ListOptions{
		Prefix: name + "/",
	}
	list := fsys.Bucket.List(listOpt)
	for {
		next, err := list.Next(ctx)
		if err != nil && !errors.Is(err, io.EOF) {
			return &fs.PathError{
				Op:   "remove",
				Path: name,
				Err:  err,
			}
		}
		if next == nil {
			break
		}
		fsys.log.DebugCtx(ctx, "remove file", "name", next.Key)
		if err := fsys.Bucket.Delete(ctx, next.Key); err != nil {
			return &fs.PathError{
				Op:   "remove",
				Path: next.Key,
				Err:  err,
			}
		}
	}
	return nil
}

func (fsys *FS) Copy(ctx context.Context, dst, src string) error {
	fsys.log.DebugCtx(ctx, "copy", "dst", dst, "src", src)
	for _, p := range []string{src, dst} {
		if !fs.ValidPath(p) {
			return &fs.PathError{
				Op:   "copy",
				Path: p,
				Err:  fs.ErrInvalid,
			}
		}
		if p == "." {
			return &fs.PathError{
				Op:   "copy",
				Path: p,
				Err:  fs.ErrInvalid,
			}
		}
	}
	return fsys.Bucket.Copy(ctx, dst, src, &blob.CopyOptions{})
}
