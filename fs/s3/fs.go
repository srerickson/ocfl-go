package s3

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"iter"
	"log/slog"

	s3mgr "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// BucketFS implements ocfl.WriteFS, ocfl.CopyFS, and ocfl.ObjectRootIterator
// for an S3 bucket.
type BucketFS struct {
	// S3 implementation (required).
	S3 S3API
	// S3 Bucket (required).
	Bucket string
	// Logger logs method calls (using the debug log level), if set
	Logger *slog.Logger

	// UploaderOptions is used to configure the upload manager
	// used in write
	UploaderOption func(*s3mgr.Uploader)

	// MultiPartCopyOption is used to conffigure multipart uploads.
	MultiPartCopyOption func(*MultiCopier)
}

func (f *BucketFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	f.debugLog(ctx, "s3:openfile", "bucket", f.Bucket, "name", name)
	return openFile(ctx, f.S3, f.Bucket, name)
}

func (f *BucketFS) DirEntries(ctx context.Context, dir string) iter.Seq2[fs.DirEntry, error] {
	f.debugLog(ctx, "s3:readdir", "bucket", f.Bucket, "name", dir)
	return dirEntries(ctx, f.S3, f.Bucket, dir)
}

func (f *BucketFS) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	f.debugLog(ctx, "s3:write", "bucket", f.Bucket, "name", name)
	if f.UploaderOption == nil {
		return write(ctx, f.S3, f.Bucket, name, r)
	}
	return write(ctx, f.S3, f.Bucket, name, r, f.UploaderOption)
}

func (f *BucketFS) Copy(ctx context.Context, dst, src string) (int64, error) {
	f.debugLog(ctx, "s3:copy", "bucket", f.Bucket, "dst", dst, "src", src)
	size, err := copy(ctx, f.S3, f.Bucket, dst, src)
	if err != nil && errors.Is(err, errCopySrcTooLarge) {
		// source is too large for basic copy -- try multipart copy
		copier := NewMultiCopier(f.S3, f.MultiPartCopyOption)
		return copier.Copy(ctx, f.Bucket, dst, src)
	}
	return size, err
}

func (f *BucketFS) Remove(ctx context.Context, name string) error {
	f.debugLog(ctx, "s3:remove", "bucket", f.Bucket, "name", name)
	return remove(ctx, f.S3, f.Bucket, name)
}

func (f *BucketFS) RemoveAll(ctx context.Context, name string) error {
	f.debugLog(ctx, "s3:remove_all", "bucket", f.Bucket, "name", name)
	return removeAll(ctx, f.S3, f.Bucket, name)
}

func (f *BucketFS) WalkFiles(ctx context.Context, dir string) iter.Seq2[*ocflfs.FileRef, error] {
	f.debugLog(ctx, "s3:walkfiles", "bucket", f.Bucket, "prefix", dir)
	files := walkFiles(ctx, f.S3, f.Bucket, dir)
	// The values yielded by walkfiles don't include the FS, we need to
	// add it here.
	return func(yield func(*ocflfs.FileRef, error) bool) {
		for file, err := range files {
			if file != nil {
				file.FS = f
			}
			if !yield(file, err) {
				break
			}
		}
	}
}

type S3API interface {
	OpenFileAPI
	ReadDirAPI
	WriteAPI
	CopyAPI
	RemoveAPI
	RemoveAllAPI
	ObjectRootsAPI
	FilesAPI
}

// OpenFileAPI includes S3 methods needed for OpenFile()
type OpenFileAPI interface {
	HeadObject(context.Context, *s3v2.HeadObjectInput, ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error)
	GetObject(context.Context, *s3v2.GetObjectInput, ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error)
}

// ReadDirAPI includes S3 methods needed for ReadDir()
type ReadDirAPI interface {
	ListObjectsV2(context.Context, *s3v2.ListObjectsV2Input, ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error)
}

// WriteAPI includes S3 methods needed for Write()
type WriteAPI interface {
	PutObject(context.Context, *s3v2.PutObjectInput, ...func(*s3v2.Options)) (*s3v2.PutObjectOutput, error)
	UploadPart(context.Context, *s3v2.UploadPartInput, ...func(*s3v2.Options)) (*s3v2.UploadPartOutput, error)
	CreateMultipartUpload(context.Context, *s3v2.CreateMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.CreateMultipartUploadOutput, error)
	CompleteMultipartUpload(context.Context, *s3v2.CompleteMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.CompleteMultipartUploadOutput, error)
	AbortMultipartUpload(context.Context, *s3v2.AbortMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.AbortMultipartUploadOutput, error)
}

// CopyAPI includes S3 methods needed for Copy()
type CopyAPI interface {
	MultiCopyAPI
	CopyObject(context.Context, *s3v2.CopyObjectInput, ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error)
}

type MultiCopyAPI interface {
	HeadObject(context.Context, *s3v2.HeadObjectInput, ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error)
	CreateMultipartUpload(context.Context, *s3v2.CreateMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.CreateMultipartUploadOutput, error)
	UploadPartCopy(context.Context, *s3v2.UploadPartCopyInput, ...func(*s3v2.Options)) (*s3v2.UploadPartCopyOutput, error)
	CompleteMultipartUpload(context.Context, *s3v2.CompleteMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.CompleteMultipartUploadOutput, error)
	AbortMultipartUpload(context.Context, *s3v2.AbortMultipartUploadInput, ...func(*s3v2.Options)) (*s3v2.AbortMultipartUploadOutput, error)
}

// RemoveAPI includes S3 methods needed for Remove()
type RemoveAPI interface {
	DeleteObject(context.Context, *s3v2.DeleteObjectInput, ...func(*s3v2.Options)) (*s3v2.DeleteObjectOutput, error)
}

// RemoveAllAPI includes S3 methods needed for RemoveAll()
type RemoveAllAPI interface {
	ListObjectsV2(context.Context, *s3v2.ListObjectsV2Input, ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error)
	DeleteObject(context.Context, *s3v2.DeleteObjectInput, ...func(*s3v2.Options)) (*s3v2.DeleteObjectOutput, error)
}

// ObjectRootsAPI includes S3 methods needed for ObjectRoots()
type ObjectRootsAPI interface {
	ListObjectsV2(context.Context, *s3v2.ListObjectsV2Input, ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error)
}

// FilesAPI includes S3 methods needed for ObjectRoots()
type FilesAPI interface {
	ListObjectsV2(context.Context, *s3v2.ListObjectsV2Input, ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error)
}

func (fs *BucketFS) debugLog(ctx context.Context, msg string, args ...any) {
	if fs.Logger != nil {
		fs.Logger.DebugContext(ctx, msg, args...)
	}
}
