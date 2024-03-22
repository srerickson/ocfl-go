package s3

import (
	"context"
	"io"
	"io/fs"
)

type FS struct {
	S3 S3API

	// S3 Bucket to read/write to
	Bucket string

	// DefaultUploadPartSize is the size in bytes for object
	// parts for multipart uploads. The default is 5MiB. If
	// the size of the upload can be determined and the part
	// size is too small to complete the upload with the max number
	// of parts, the part size is increased in 1 MiB increments.
	DefaultUploadPartSize int64
	// UploadConcurrency sets the number of goroutines per
	// upload for sending object parts. The default it 5.
	UploadConcurrency int

	// DefaultCopyPartSize sets the size of the object parts used
	// for multipart object copy. If the part size is too
	// small to be copied using the max number of parts,
	// the part size will be increased in 1 MiB increments
	// until it fits.
	DefaultCopyPartSize int64
	// CopyPartConcurrency stes the number of gourites
	// per copy for copying object parts. defaults to 12.
	CopyPartConcurrency int
}

type S3API interface {
	OpenFileAPI
	ReadDirAPI
	WriteAPI
	CopyAPI
	RemoveAPI
	RemoveAllAPI
}

func (f *FS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	return OpenFile(ctx, f.S3, f.Bucket, name)
}

func (f *FS) ReadDir(ctx context.Context, dir string) ([]fs.DirEntry, error) {
	return ReadDir(ctx, f.S3, f.Bucket, dir)
}
func (f *FS) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	return Write(ctx, f.S3, f.Bucket, f.UploadConcurrency, f.DefaultUploadPartSize, name, r)
}

func (f *FS) Copy(ctx context.Context, dst, src string) error {
	return Copy(ctx, f.S3, f.Bucket, f.CopyPartConcurrency, f.DefaultCopyPartSize, dst, src)
}

func (f *FS) Remove(ctx context.Context, name string) error {
	return Remove(ctx, f.S3, f.Bucket, name)
}

func (f *FS) RemoveAll(ctx context.Context, name string) error {
	return RemoveAll(ctx, f.S3, f.Bucket, name)
}
