package s3

import (
	"context"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// BucketFS implements ocfl.WriteFS, ocfl.CopyFS, and ocfl.ObjectRootIterator
// for an S3 bucket.
type BucketFS struct {
	client               S3API
	bucket               string
	logger               *slog.Logger
	uploader             *transfermanager.Client
	uploaderOptions      []func(*transfermanager.Options)
	multiPartCopyOptions []func(*MultiCopier)
}

// NewBucketFS returns a new *BucketFS for the given bucket
func NewBucketFS(client S3API, bucket string, opts ...func(*BucketFS)) *BucketFS {
	fsys := &BucketFS{
		client: client,
		bucket: bucket,
	}
	for _, o := range opts {
		if o != nil {
			o(fsys)
		}
	}
	fsys.uploader = transfermanager.New(client, fsys.uploaderOptions...)
	return fsys
}

// WithLogger sets a logger which is used to send debug-level log messages for
// s3 requests.
func WithLogger(logger *slog.Logger) func(*BucketFS) {
	return func(bf *BucketFS) {
		bf.logger = logger
	}
}

// WithUploaderOptions sets options used to create the s3 transfer manager
// client used to write files.
func WithUploaderOptions(opts ...func(*transfermanager.Options)) func(*BucketFS) {
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
	return f.client
}

// Bucket returns the bucket used to create f.
func (f *BucketFS) Bucket() string {
	return f.bucket
}

// OpenFile opens the object at name for reading, per [ocflfs.FS].
//
// A HEAD request resolves the object's size and mtime up front, so Stat,
// Read and Seek never need another round trip to answer with them. A store
// that omits ContentLength or LastModified from its HEAD response is
// refused here rather than producing a File whose methods can't answer.
func (f *BucketFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	f.debugLog(ctx, "s3:openfile", "bucket", f.bucket, "name", name)
	return openFile(ctx, f.client, f.bucket, name)
}

// DirEntries lists the immediate children of dir, per [ocflfs.DirEntriesFS].
//
// Directories are reconstructed from key prefixes, so a prefix that matches
// no object is reported as fs.ErrNotExist: an empty directory is not
// something S3 can hold. The one exception is dir ".", which names the
// bucket and exists whether or not it holds any objects -- an empty bucket
// lists empty, as an empty root directory does on the local backend.
//
// The listing is paged (ListObjectsV2, maxKeys per page); a listing that
// fails partway yields the entries already fetched and then the error,
// matching [ocflfs.DirEntriesFS].
func (f *BucketFS) DirEntries(ctx context.Context, dir string) iter.Seq2[fs.DirEntry, error] {
	f.debugLog(ctx, "s3:readdir", "bucket", f.bucket, "name", dir)
	return dirEntries(ctx, f.client, f.bucket, dir)
}

func (f *BucketFS) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	return f.WriteWithOptions(ctx, name, r)
}

// WriteWithOptions writes the contents of r to name, applying opts to the
// UploadObjectInput first. It is [BucketFS.Write] with access to the rest of
// the request: a storage class, server-side encryption, a conditional put.
//
// Setting a ContentLength through an option is neither necessary nor usually
// wanted, and does not declare the request's Content-Length: the transfer
// manager buffers r into chunks of its own, and the SDK derives each request's
// Content-Length from the bytes it is actually sending. The value serves only
// as a size hint, used to raise the part size when the object would otherwise
// need more parts than the upload allows. A wrong one is not an error.
//
// Bucket, Key and Body are set after opts run, so an option cannot redirect
// the write.
func (f *BucketFS) WriteWithOptions(ctx context.Context, name string, r io.Reader, opts ...func(*transfermanager.UploadObjectInput)) (int64, error) {
	f.debugLog(ctx, "s3:write", "bucket", f.bucket, "name", name)
	return write(ctx, f.uploader, f.bucket, name, r, opts...)
}

// Copy copies the object at src to dst within the bucket, per [ocflfs.CopyFS].
//
// The strategy is chosen from src's HEAD ContentLength, not by trying a copy
// and inspecting the failure: a source of 5 GiB or less is copied with one
// CopyObject request, and a larger one is copied part by part with
// [MultiCopier], tunable through [WithMultiPartCopyOption]. A store that
// enforces a smaller CopyObject limit than 5 GiB returns that store's error
// rather than falling back to MultiCopier.
func (f *BucketFS) Copy(ctx context.Context, dst, src string) (int64, error) {
	f.debugLog(ctx, "s3:copy", "bucket", f.bucket, "dst", dst, "src", src)
	return copy(ctx, f.client, f.bucket, dst, src, f.multiPartCopyOptions...)
}

// Remove deletes the object at name, per [ocflfs.WriteFS].
//
// Removing a key that is not in the bucket returns an error satisfying
// errors.Is(err, fs.ErrNotExist), which S3 does not give for free: DeleteObject
// answers 204 whether or not the key was there. Remove therefore issues a
// HeadObject first, so it costs two round trips rather than one, and the pair
// is not atomic -- a key deleted by another writer between the HEAD and the
// DELETE is reported as removed rather than missing.
//
// Name "." is rejected with fs.ErrInvalid without issuing any request: it
// names the bucket, not an object.
func (f *BucketFS) Remove(ctx context.Context, name string) error {
	f.debugLog(ctx, "s3:remove", "bucket", f.bucket, "name", name)
	return remove(ctx, f.client, f.bucket, name)
}

// RemoveAll deletes every object under name, per [ocflfs.WriteFS]. Name "."
// empties the bucket. A name that matches nothing is not an error.
//
// Keys are listed a page at a time and each page is deleted in as few
// DeleteObjects requests as the API allows, so removing an OCFL object costs
// a request per thousand files rather than one per file.
//
// Deletion is best-effort: a key that will not delete does not abandon the
// keys after it, on that page or on any later one. Only a failed listing
// stops the walk, since without one there is nothing left to attempt. The
// returned error joins every failure, naming the key each one is about --
// including the per-key failures DeleteObjects reports in an otherwise
// successful response.
func (f *BucketFS) RemoveAll(ctx context.Context, name string) error {
	f.debugLog(ctx, "s3:remove_all", "bucket", f.bucket, "name", name)
	return removeAll(ctx, f.client, f.bucket, name)
}

// SameBackend implements [ocflfs.SameBackend] for f. It reports true if other
// is a *BucketFS naming the same bucket, using the same client. Client
// identity is checked with reflect: pointer-like clients (the common case)
// compare by pointer, and any other comparable client type falls back to ==;
// a client whose dynamic type isn't comparable (e.g. a struct value with a
// map field) reports false rather than risk a panic comparing it directly.
func (f *BucketFS) SameBackend(other ocflfs.FS) bool {
	o, ok := other.(*BucketFS)
	if !ok || o.bucket != f.bucket {
		return false
	}
	return sameClient(f.client, o.client)
}

func sameClient(a, b S3API) bool {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if !va.IsValid() || !vb.IsValid() || va.Type() != vb.Type() {
		return false
	}
	switch va.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return va.Pointer() == vb.Pointer()
	}
	if va.Comparable() {
		return va.Interface() == vb.Interface()
	}
	return false
}

// WalkFiles returns an iterator over every object under dir, per
// [ocflfs.FileWalker].
//
// Each yielded ref carries this BucketFS as its FS, so a caller can open the
// file through the ref alone. Iteration stops as soon as the callback returns
// false, and a listing failure is delivered as the pair's error element with
// nothing yielded after it.
func (f *BucketFS) WalkFiles(ctx context.Context, dir string) iter.Seq2[*ocflfs.FileRef, error] {
	f.debugLog(ctx, "s3:walkfiles", "bucket", f.bucket, "prefix", dir)
	return walkFiles(ctx, f.client, f.bucket, dir, f)
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

// RemoveAPI includes S3 methods needed for Remove().
//
// HeadObject is here because DeleteObject is idempotent and cannot report a
// key that was never there, which the WriteFS.Remove contract requires. See
// [BucketFS.Remove].
type RemoveAPI interface {
	HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// RemoveAllAPI includes S3 methods needed for RemoveAll().
//
// The delete is DeleteObjects, not DeleteObject: RemoveAll deletes a whole
// listing page per request rather than one key per request. See
// [BucketFS.RemoveAll].
type RemoveAllAPI interface {
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	DeleteObjects(context.Context, *s3.DeleteObjectsInput, ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
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
