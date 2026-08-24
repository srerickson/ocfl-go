package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Compile-time check that s3File implements io.Seeker
var _ io.Seeker = (*s3File)(nil)

func openFile(ctx context.Context, api OpenFileAPI, buck string, name string, logger *slog.Logger) (fs.File, error) {
	if !fs.ValidPath(name) || name == "." {
		return nil, pathErr("open", name, fs.ErrInvalid)
	}
	headIn := &s3.HeadObjectInput{Bucket: &buck, Key: &name}
	headOut, err := api.HeadObject(ctx, headIn)
	if err != nil {
		fsErr := &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  err,
		}
		if errIsNotExist(err) {
			fsErr.Err = notExistErr(err)
		}
		return nil, fsErr
	}
	if headOut.ContentLength == nil {
		// S3-compatible stores and proxies may omit Content-Length on
		// HEAD responses. The object size is required for Stat, Read,
		// and Seek below, so refuse to open rather than carry an unknown
		// length. Mirrors the guard in MultiCopier.Copy.
		return nil, pathErr("open", name, errors.New("missing content length"))
	}
	f := &s3File{
		ctx:    ctx,
		api:    api,
		bucket: buck,
		key:    name,
		info:   headOut,
		logger: logger,
	}
	return f, nil
}

// s3File implements fs.File and io.Seeker.
//
// Like *os.File and the fs.File values returned by other backends, an s3File
// is NOT safe for concurrent use. Read, Seek and Close all mutate the shared
// body reader and file offset, so callers that share one handle across
// goroutines must provide their own synchronization. Open a separate file
// handle per goroutine instead; each carries its own body and offset.
type s3File struct {
	ctx    context.Context
	api    OpenFileAPI
	bucket string
	key    string
	body   io.ReadCloser
	info   *s3.HeadObjectOutput
	offset int64 // current position in the file
	logger *slog.Logger
}

func (f *s3File) Stat() (fs.FileInfo, error) {
	return &iofsInfo{
		name:    path.Base(f.key),
		size:    *f.info.ContentLength,
		mode:    fileMode,
		modTime: *f.info.LastModified,
		sys:     f.info,
	}, nil
}

func (f *s3File) Read(p []byte) (int, error) {
	size := *f.info.ContentLength
	if f.offset >= size {
		return 0, io.EOF
	}
	if f.body == nil {
		params := &s3.GetObjectInput{
			Bucket: &f.bucket,
			Key:    &f.key,
			// ensure unchanged since open
			IfMatch:           f.info.ETag,
			IfUnmodifiedSince: f.info.LastModified,
		}
		if f.offset > 0 {
			rangeStr := fmt.Sprintf("bytes=%d-", f.offset)
			params.Range = &rangeStr
		}
		obj, err := f.api.GetObject(f.ctx, params)
		if err != nil {
			return 0, err
		}
		f.body = obj.Body
	}
	n, err := f.body.Read(p)
	f.offset += int64(n)
	return n, err
}

func (f *s3File) Close() error {
	if f.body == nil {
		return nil
	}
	return f.body.Close()
}

func (f *s3File) Name() string {
	return path.Base(f.key)
}

// Seek implements io.Seeker. It repositions the file offset for the next Read.
// Seeking to a new position invalidates any existing body reader: it is closed
// and discarded so the next Read issues a fresh GetObject with the appropriate
// Range header. A seek to the current position keeps the body, so sequential
// readers that "seek" to where they already are do not pay for a new request.
//
// A body Close error does not fail the Seek, but it may mean a connection
// leaked, so it is logged at debug level when the file has a logger.
func (f *s3File) Seek(offset int64, whence int) (int64, error) {
	size := *f.info.ContentLength
	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = f.offset + offset
	case io.SeekEnd:
		newOffset = size + offset
	default:
		return 0, errors.New("s3: invalid whence")
	}
	if newOffset < 0 {
		return 0, errors.New("s3: negative position")
	}
	if newOffset != f.offset {
		if f.body != nil {
			if err := f.body.Close(); err != nil && f.logger != nil {
				f.logger.DebugContext(f.ctx, "s3:seek:close", "bucket", f.bucket, "key", f.key, "error", err)
			}
			f.body = nil
		}
		f.offset = newOffset
	}
	return newOffset, nil
}
