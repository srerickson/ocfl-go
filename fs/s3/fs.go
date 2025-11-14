package s3

import (
	"context"
	"io"
	"io/fs"
	"iter"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// BucketFS implements ocfl.WriteFS, ocfl.CopyFS, and ocfl.ObjectRootIterator
// for an S3 bucket.
type BucketFS struct {
	s3api                S3API  // s3api implementation (required).
	bucket               string // S3 bucket (required).
	logger               *slog.Logger
	uploader             *manager.Uploader
	uploaderOptions      []func(*manager.Uploader)
	multiPartCopyOptions []func(*MultiCopier)
}

// NewBucketFS returns a new *BucketFS for the given bucket
func NewBucketFS(client S3API, bucket string, opts ...func(*BucketFS)) *BucketFS {
	fsys := &BucketFS{
		s3api:  client,
		bucket: bucket,
	}
	for _, o := range opts {
		if o != nil {
			o(fsys)
		}
	}
	fsys.uploader = manager.NewUploader(client, fsys.uploaderOptions...)
	return fsys
}

// WithLogger sets a logger which is used to send debug-level log messages for
// s3 requests.
func WithLogger(logger *slog.Logger) func(*BucketFS) {
	return func(bf *BucketFS) {
		bf.logger = logger
	}
}

// WithUploaderOptions sets options used to create the s3 manager.Uploader used
// write files.
func WithUploaderOptions(opts ...func(*manager.Uploader)) func(*BucketFS) {
	return func(bf *BucketFS) {
		bf.uploaderOptions = opts
	}
}

func WithMultiPartCopyOption(opts ...func(*MultiCopier)) func(*BucketFS) {
	return func(bf *BucketFS) {
		bf.multiPartCopyOptions = opts
	}
}

// Client returns the S3 API client used to create f
func (f *BucketFS) Client() S3API {
	return f.s3api
}

// Bucket returns the bucket used to create f.
func (f *BucketFS) Bucket() string {
	return f.bucket
}

func (f *BucketFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	f.debugLog(ctx, "s3:openfile", "bucket", f.bucket, "name", name)
	return openFile(ctx, f.s3api, f.bucket, name)
}

func (f *BucketFS) DirEntries(ctx context.Context, dir string) iter.Seq2[fs.DirEntry, error] {
	f.debugLog(ctx, "s3:readdir", "bucket", f.bucket, "name", dir)
	return dirEntries(ctx, f.s3api, f.bucket, dir)
}

func (f *BucketFS) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	f.debugLog(ctx, "s3:write", "bucket", f.bucket, "name", name)
	return write(ctx, f.uploader, f.bucket, name, r)
}

func (f *BucketFS) Copy(ctx context.Context, dst, src string) (int64, error) {
	f.debugLog(ctx, "s3:copy", "bucket", f.bucket, "dst", dst, "src", src)
	return copy(ctx, f.s3api, f.bucket, dst, src, f.multiPartCopyOptions...)
}

func (f *BucketFS) Remove(ctx context.Context, name string) error {
	f.debugLog(ctx, "s3:remove", "bucket", f.bucket, "name", name)
	return remove(ctx, f.s3api, f.bucket, name)
}

func (f *BucketFS) RemoveAll(ctx context.Context, name string) error {
	f.debugLog(ctx, "s3:remove_all", "bucket", f.bucket, "name", name)
	return removeAll(ctx, f.s3api, f.bucket, name)
}

func (f *BucketFS) WalkFiles(ctx context.Context, dir string) iter.Seq2[*ocflfs.FileRef, error] {
	f.debugLog(ctx, "s3:walkfiles", "bucket", f.bucket, "prefix", dir)
	files := walkFiles(ctx, f.s3api, f.bucket, dir)
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
	HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// ReadDirAPI includes S3 methods needed for ReadDir()
type ReadDirAPI interface {
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// WriteAPI includes S3 methods needed for Write()
type WriteAPI interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	UploadPart(context.Context, *s3.UploadPartInput, ...func(*s3.Options)) (*s3.UploadPartOutput, error)
	CreateMultipartUpload(context.Context, *s3.CreateMultipartUploadInput, ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error)
	CompleteMultipartUpload(context.Context, *s3.CompleteMultipartUploadInput, ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error)
	AbortMultipartUpload(context.Context, *s3.AbortMultipartUploadInput, ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error)
}

// CopyAPI includes S3 methods needed for Copy()
type CopyAPI interface {
	MultiCopyAPI
	CopyObject(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
}

type MultiCopyAPI interface {
	HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	CreateMultipartUpload(context.Context, *s3.CreateMultipartUploadInput, ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error)
	UploadPartCopy(context.Context, *s3.UploadPartCopyInput, ...func(*s3.Options)) (*s3.UploadPartCopyOutput, error)
	CompleteMultipartUpload(context.Context, *s3.CompleteMultipartUploadInput, ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error)
	AbortMultipartUpload(context.Context, *s3.AbortMultipartUploadInput, ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error)
}

// RemoveAPI includes S3 methods needed for Remove()
type RemoveAPI interface {
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// RemoveAllAPI includes S3 methods needed for RemoveAll()
type RemoveAllAPI interface {
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// ObjectRootsAPI includes S3 methods needed for ObjectRoots()
type ObjectRootsAPI interface {
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

// FilesAPI includes S3 methods needed for ObjectRoots()
type FilesAPI interface {
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

func (fs *BucketFS) debugLog(ctx context.Context, msg string, args ...any) {
	if fs.logger != nil {
		fs.logger.DebugContext(ctx, msg, args...)
	}
}
