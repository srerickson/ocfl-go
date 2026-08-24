package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

const (
	megabyte          int64 = 1024 * 1024
	partSizeIncrement       = 1 * megabyte

	// maxCopySize is the maximum size of a source object that can be copied
	// with a single CopyObject request. Larger objects must be copied in
	// parts using MultiCopier.
	maxCopySize int64 = 5 * 1024 * 1024 * 1024

	// modes retured by Stat()
	fileMode = 0644 | fs.ModeIrregular
	dirMode  = 0755 | fs.ModeDir
)

var (
	// these are variable because we need pass them as pointers
	delim         = "/"
	maxKeys int32 = 1000
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
			fsErr.Err = fs.ErrNotExist
		}
		return nil, fsErr
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

// dirEntries implements directory listing: the emulation of fs.ReadDir on
// S3's flat key space. It is the S3 half of the backend readdir contract
// pinned by fs/s3/direntries_test.go and fs/local/localfs_test.go; read the
// two together.
//
// # S3 has no directories
//
// S3 stores only objects; a "directory" is an emergent property of key
// prefixes (a prefix with a trailing "/" used with a Delimiter), and there
// is no way to create an empty one. Every branch below follows from that:
//
//   - A prefix that has objects or deeper common prefixes lists them as
//     file entries (fileMode) and subdirectory entries (dirMode),
//     respectively.
//   - A non-root prefix with neither objects nor common prefixes is
//     indistinguishable from a path that never existed — S3 offers no
//     "empty directory" object to tell the two apart — so it is reported as
//     missing with fs.ErrNotExist, matching the local backend's readdir of
//     a missing directory.
//   - The root, dir=".", is the one prefix that is always known to exist:
//     it is the bucket itself, and a *missing* bucket surfaces as a
//     ListObjectsV2 error rather than as an empty listing. On a bucket with
//     no keys at all, "." therefore yields zero entries and no error,
//     matching the local backend's readdir of an existing but empty
//     directory (fs/local/localfs_test.go, "empty top-level directory
//     returns zero entries").
//
// # Why the asymmetry is deliberate
//
// The two backends still disagree on empty *non-root* prefixes: a local
// empty directory reads back as an empty listing, while an S3 prefix that
// would occupy the same position reads back as fs.ErrNotExist. This is a
// faithful emulation, not a bug: local storage can represent emptiness, S3
// cannot, and a valid OCFL object never depends on an empty directory —
// every OCFL version directory contains at least inventory.json, and OCFL
// storage-root and object layouts do not create empty directories. OCFL
// extensions or third-party tooling may create empty directory-like
// prefixes on local storage; when an object is moved to S3 the same path
// simply reads as missing, which reflects the key space it now lives in.
//
// The root case is the only one where backends must agree, because
// storage-root scanning (Root.NewRoot in root.go) and ocflfs.RemoveAll(".")
// (fs/fs.go) both start by reading dir="." : an empty bucket is a valid
// (new) storage root and must read back as an empty directory, never as
// fs.ErrNotExist. (NewRoot in root.go happens to tolerate fs.ErrNotExist,
// but ocflfs.RemoveAll(".") does not — on an empty bucket it would
// otherwise fail instead of being the no-op it is on local storage — and
// "empty" is simply the honest answer for a prefix that is guaranteed to
// exist.) Root-empty behavior is pinned by TestDirEntries_RootEmptyBucket
// in fs/s3/direntries_test.go.
func dirEntries(ctx context.Context, api ReadDirAPI, buck string, dir string) iter.Seq2[fs.DirEntry, error] {
	return func(yield func(fs.DirEntry, error) bool) {
		if !fs.ValidPath(dir) {
			yield(nil, pathErr("readdir", dir, fs.ErrInvalid))
			return
		}
		params := &s3.ListObjectsV2Input{
			Bucket:    &buck,
			Delimiter: &delim,
			MaxKeys:   &maxKeys,
		}
		if dir != "." {
			params.Prefix = aws.String(dir + "/")
		}
		prefixHasContent := false
		for {
			list, err := api.ListObjectsV2(ctx, params)
			if err != nil {
				yield(nil, pathErr("readdir", dir, err))
				return
			}
			numDirs := len(list.CommonPrefixes)
			numFiles := len(list.Contents)
			numEntries := numDirs + numFiles
			if numEntries == 0 {
				if !prefixHasContent && dir != "." {
					// treat prefix without objects as a missing directory.
					// The root (dir=".") is exempt: "." names the bucket
					// itself, which always exists, so an empty bucket reads
					// as an empty directory rather than a missing path.
					yield(nil, pathErr("readdir", dir, fs.ErrNotExist))
				}
				return
			}
			prefixHasContent = true
			entries := make([]fs.DirEntry, numEntries)
			for i, item := range list.CommonPrefixes {
				entries[i] = &iofsInfo{
					name: path.Base(*item.Prefix),
					mode: dirMode,
				}
			}
			for i, item := range list.Contents {
				entries[numDirs+i] = &iofsInfo{
					name:    path.Base(*item.Key),
					size:    *item.Size,
					mode:    fileMode,
					modTime: *item.LastModified,
					//sys:     &item,
				}
			}
			slices.SortFunc(entries, func(a, b fs.DirEntry) int {
				return strings.Compare(a.Name(), b.Name())
			})
			for _, entry := range entries {
				if !yield(entry, nil) {
					return
				}
			}
			params.ContinuationToken = list.NextContinuationToken
			if params.ContinuationToken == nil {
				break
			}
		}
	}

}

func write(ctx context.Context, uploader *manager.Uploader, buck string, key string, r io.Reader, opts ...func(*s3.PutObjectInput)) (int64, error) {
	if !fs.ValidPath(key) || key == "." {
		return 0, pathErr("write", key, fs.ErrInvalid)
	}
	countReader := &countReader{Reader: r}
	var putInput s3.PutObjectInput
	for _, o := range opts {
		if o != nil {
			o(&putInput)
		}
	}
	putInput.Bucket = &buck
	putInput.Key = &key
	putInput.Body = countReader
	if putInput.ContentLength == nil {
		// try to get content length from r
		size := int64(-1)
		switch val := r.(type) {
		case fs.File:
			if info, err := val.Stat(); err == nil {
				size = info.Size()
			}
		case *bytes.Reader:
			size = val.Size()
		case *io.LimitedReader:
			size = val.N
		case io.Seeker:
			// Generic seekable reader (e.g. *os.File, *strings.Reader):
			// determine the length by seeking to the end, recording the
			// offset, and restoring the original position. If any seek
			// fails, leave ContentLength nil as before.
			if cur, err := val.Seek(0, io.SeekCurrent); err == nil {
				if end, err := val.Seek(0, io.SeekEnd); err == nil {
					if _, err := val.Seek(cur, io.SeekStart); err == nil {
						size = end
					}
				}
			}
		}
		if size > -1 {
			putInput.ContentLength = &size
		}
	}
	if _, err := uploader.Upload(ctx, &putInput); err != nil {
		return 0, &fs.PathError{Op: "write", Path: key, Err: err}
	}
	return countReader.size, nil
}

func copy(ctx context.Context, api CopyAPI, buck string, dst, src string, opts ...func(*MultiCopier)) (int64, error) {
	if !fs.ValidPath(src) || src == "." {
		return 0, pathErr("copy", src, fs.ErrInvalid)
	}
	if !fs.ValidPath(dst) || dst == "." {
		return 0, pathErr("copy", dst, fs.ErrInvalid)
	}
	srcHead, err := api.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &buck,
		Key:    &src,
	})
	if err != nil {
		fsErr := &fs.PathError{
			Op:   "copy",
			Path: src,
			Err:  err,
		}
		if errIsNotExist(err) {
			fsErr.Err = fs.ErrNotExist
		}
		return 0, fsErr
	}
	escapedSrc := url.QueryEscape(buck + "/" + src)
	params := &s3.CopyObjectInput{
		Bucket:     &buck,
		CopySource: &escapedSrc, // value must be URL-encoded
		Key:        &dst,
	}
	if *srcHead.ContentLength > maxCopySize {
		// source object is too large for a single CopyObject request:
		// use multipart copy.
		return NewMultiCopier(api, opts...).Copy(ctx, buck, dst, src, srcHead)
	}
	if _, err := api.CopyObject(ctx, params); err != nil {
		return 0, pathErr("copy", src, err)
	}
	return *srcHead.ContentLength, nil
}

func remove(ctx context.Context, api RemoveAPI, b string, name string) error {
	if !fs.ValidPath(name) {
		return pathErr("remove", name, fs.ErrInvalid)
	}
	if name == "." {
		return pathErr("remove", name, fs.ErrNotExist)
	}
	// Contract (WriteFS.Remove in fs/fs.go): removing a missing file must
	// return an error satisfying errors.Is(err, fs.ErrNotExist). S3's
	// DeleteObject is idempotent — it succeeds (204) even for missing keys —
	// so probe existence with HeadObject first and map a not-found HEAD to
	// fs.ErrNotExist before deleting.
	if _, err := api.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &b,
		Key:    aws.String(name),
	}); err != nil {
		if errIsNotExist(err) {
			return pathErr("remove", name, fs.ErrNotExist)
		}
		return pathErr("remove", name, err)
	}
	if _, err := api.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &b,
		Key:    aws.String(name),
	}); err != nil {
		return pathErr("remove", name, err)
	}
	return nil
}

func removeAll(ctx context.Context, api RemoveAllAPI, buck string, name string) error {
	if !fs.ValidPath(name) {
		return pathErr("removeall", name, fs.ErrInvalid)
	}
	params := &s3.ListObjectsV2Input{Bucket: &buck, MaxKeys: &maxKeys}
	if name != "." {
		params.Prefix = aws.String(name + "/")
	}
	for {
		list, err := api.ListObjectsV2(ctx, params)
		if err != nil {
			return pathErr("removeall", name, err)
		}
		// Delete each page of listed objects with a single batch
		// DeleteObjects request.
		if len(list.Contents) > 0 {
			identifiers := make([]types.ObjectIdentifier, 0, len(list.Contents))
			for _, obj := range list.Contents {
				identifiers = append(identifiers, types.ObjectIdentifier{Key: obj.Key})
			}
			if _, err := api.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: &buck,
				Delete: &types.Delete{Objects: identifiers},
			}); err != nil {
				return pathErr("removeall", name, err)
			}
		}
		params.ContinuationToken = list.NextContinuationToken
		if params.ContinuationToken == nil {
			break
		}
	}
	return nil
}

// walkFiles returns an iterator that yields PathInfo for files in the dir
func walkFiles(ctx context.Context, api FilesAPI, buck string, dir string) iter.Seq2[*ocflfs.FileRef, error] {
	return func(yield func(*ocflfs.FileRef, error) bool) {
		const op = "list_files"
		if !fs.ValidPath(dir) {
			yield(nil, pathErr(op, dir, fs.ErrInvalid))
			return
		}
		params := &s3.ListObjectsV2Input{
			Bucket:  &buck,
			MaxKeys: &maxKeys,
		}
		if dir != "." {
			params.Prefix = aws.String(dir + "/")
		}
		for {
			listPage, err := api.ListObjectsV2(ctx, params)
			if err != nil {
				yield(nil, pathErr(op, dir, err))
				return
			}
			for _, s3obj := range listPage.Contents {
				refPath := *s3obj.Key
				if dir != "." {
					refPath = strings.TrimPrefix(refPath, dir+"/")
				}
				info := &ocflfs.FileRef{
					BaseDir: dir,
					Path:    refPath,
					Info: &iofsInfo{
						name:    path.Base(*s3obj.Key),
						size:    *s3obj.Size,
						mode:    fileMode,
						modTime: *s3obj.LastModified,
					},
				}
				if !yield(info, nil) {
					return
				}
			}
			params.ContinuationToken = listPage.NextContinuationToken
			if params.ContinuationToken == nil {
				break
			}
		}
	}
}

// s3File implements fs.File and io.Seeker.
//
// s3File is safe for concurrent use: the mu mutex guards the mutable body
// and offset fields across Read, Seek, and Close. Without it, concurrent
// calls could corrupt the offset and issue overlapping GetObject requests.
type s3File struct {
	// mu guards body and offset, which are mutated by Read and Seek.
	mu sync.Mutex

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
	f.mu.Lock()
	defer f.mu.Unlock()
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
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.body == nil {
		return nil
	}
	return f.body.Close()
}

func (f *s3File) Name() string {
	return path.Base(f.key)
}

// Seek implements io.Seeker. It repositions the file offset for the next Read.
// The body reader is closed and reset (and the offset updated) atomically
// under f.mu, so a concurrent Read never observes a stale body positioned at
// a different offset; the next Read issues a new GetObject request with the
// appropriate Range header.
func (f *s3File) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	// Close existing body if position changed. The old body is discarded
	// either way, so a close error does not fail the Seek, but it may mean
	// a connection leaked, so log it to make it visible.
	if f.body != nil && newOffset != f.offset {
		if err := f.body.Close(); err != nil && f.logger != nil {
			f.logger.DebugContext(f.ctx, "s3:seek:close", "bucket", f.bucket, "key", f.key, "error", err)
		}
		f.body = nil
	}
	f.offset = newOffset
	return f.offset, nil
}

// iofsInfo implements fs.FileInfo and fs.DirEntry
type iofsInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	sys     any
}

// iofsInfo implements fs.FileInfo
func (i iofsInfo) Name() string       { return i.name }
func (i iofsInfo) Size() int64        { return i.size }
func (i iofsInfo) Mode() fs.FileMode  { return i.mode }
func (i iofsInfo) ModTime() time.Time { return i.modTime }
func (i iofsInfo) IsDir() bool        { return i.mode.IsDir() }
func (i iofsInfo) Sys() any           { return i.sys }

// iofsInfo implements fs.DirEntry
func (i iofsInfo) Info() (fs.FileInfo, error) { return i, nil }
func (i iofsInfo) Type() fs.FileMode          { return i.mode.Type() }

// countReader is a reader that updates a size counter with each read.
type countReader struct {
	io.Reader
	size int64
}

func (r *countReader) Read(p []byte) (int, error) {
	s, err := r.Reader.Read(p)
	r.size += int64(s)
	return s, err
}

// pathErr makes fs.PathError errors
func pathErr(op string, path string, err error) error {
	return &fs.PathError{Op: op, Path: path, Err: err}
}

// adjustPartSize returns an adjusted partsize and part count for transfering
// totalSize in under maxParts parts using the initial partSize.
func adjustPartSize(totalSize, initialPartSize int64, maxParts int32) (psize int64, pcount int32) {
	psize = initialPartSize
	for {
		pcount = int32(totalSize / psize)
		if pcount < maxParts {
			break
		}
		psize += partSizeIncrement
	}
	if totalSize%psize > 0 {
		pcount++
	}
	return
}

func byteRange(partNum int32, partSize, totalSize int64) string {
	// aws: The range of bytes to copy from the source object. The range value must
	// use the form bytes=first-last, where the first and last are the zero-based byte
	// offsets to copy. For example, bytes=0-9 indicates that you want to copy the
	// first 10 bytes of the source. You can copy a range only if the source object is
	// greater than 5 MB.
	start := (int64(partNum) - 1) * partSize
	end := int64(partNum)*partSize - 1
	if max := totalSize - 1; end > max {
		end = max
	}
	return fmt.Sprintf("bytes=%d-%d", start, end)
}

func errIsNotExist(err error) bool {
	var notFoundErr *types.NotFound
	if errors.As(err, &notFoundErr) {
		return true
	}
	var noKeyErr *types.NoSuchKey
	return errors.As(err, &noKeyErr)
}
