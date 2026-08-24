package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
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

// sniffSeekerLength returns the number of bytes remaining from the
// reader's current position to its end (end offset minus current
// offset). It restores the original position before returning, so a
// successful sniff leaves the caller's reader unmoved.
//
// Determining the length of a generic io.Seeker requires seeking to the
// end, which temporarily moves the reader. The original position is
// always restored before a length is returned; if the restore seek
// fails the reader is left at an unknown position (typically EOF) and
// an upload would silently deliver an empty or truncated body, so this
// returns an error instead of a length and the caller must not upload.
// A probe that fails without moving the reader (or whose position can
// still be restored) returns (-1, nil) so the caller leaves
// ContentLength unset and the SDK streams the body without a declared
// length.
func sniffSeekerLength(seeker io.Seeker) (int64, error) {
	cur, err := seeker.Seek(0, io.SeekCurrent)
	if err != nil {
		return -1, nil
	}
	end, err := seeker.Seek(0, io.SeekEnd)
	if err != nil {
		// The offset is unspecified after a failed seek; try to get
		// back to the original position before giving up on the length.
		if _, rerr := seeker.Seek(cur, io.SeekStart); rerr != nil {
			return -1, fmt.Errorf("restore reader position %d after failed end-seek: %w", cur, rerr)
		}
		return -1, nil
	}
	if _, err := seeker.Seek(cur, io.SeekStart); err != nil {
		return -1, fmt.Errorf("restore reader position %d after length sniff: %w", cur, err)
	}
	if end < cur {
		return -1, nil
	}
	return end - cur, nil
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
		case *io.LimitedReader:
			size = val.N
		case io.Seeker:
			// Generic seekable reader (e.g. *os.File, *strings.Reader,
			// *bytes.Reader, or an fs.File that is also an io.Seeker such as
			// the file handle returned by Open): determine the REMAINING
			// length (end - current offset). A partially-consumed reader
			// (e.g. a strings.Reader after a first write, a *bytes.Reader
			// after a read, or a file read up to some offset) must report
			// only the bytes left, not the total size. *bytes.Reader used to
			// be special-cased with val.Size(), which reports the total size
			// of the underlying slice even for a partially-consumed reader,
			// so it is handled here like any other seeker.
			//
			// Sniffing a generic seeker requires temporarily seeking to
			// the end, so the reader must not be shared with other
			// goroutines for the duration of the write. The original
			// position is always restored before the length is used; if
			// the restore fails the reader would be left at EOF and the
			// upload would silently deliver an empty body, so write()
			// returns an error instead. If a probe fails without moving
			// the reader, ContentLength stays nil and the body is
			// streamed without a declared length.
			if n, err := sniffSeekerLength(val); err != nil {
				return 0, pathErr("write", key, err)
			} else if n >= 0 {
				size = n
			}
		case fs.File:
			// Non-seekable fs.File: fall back to the file's total size.
			if info, err := val.Stat(); err == nil {
				size = info.Size()
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
			fsErr.Err = notExistErr(err)
		}
		return 0, fsErr
	}
	if srcHead.ContentLength == nil {
		// S3-compatible stores/proxies may omit Content-Length on HEAD;
		// the object size is required to pick a copy strategy and to
		// report the copied size. Mirror the guard in MultiCopier.Copy.
		return 0, pathErr("copy", src, errors.New("missing content length"))
	}
	escapedSrc := copySourcePath(buck, src)
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
			return pathErr("remove", name, notExistErr(err))
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
			out, err := api.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: &buck,
				Delete: &types.Delete{Objects: identifiers},
			})
			if err != nil {
				return pathErr("removeall", name, err)
			}
			// S3 returns HTTP 200 even when individual keys in the batch
			// fail to delete; the per-key failures are reported in the
			// response body's Errors list. A successful API call therefore
			// does not by itself mean all objects were deleted, so surface
			// any partial failures as a joined PathError.
			if out != nil && len(out.Errors) > 0 {
				collected := make([]error, 0, len(out.Errors))
				for _, e := range out.Errors {
					collected = append(collected,
						fmt.Errorf("key %q: %s", aws.ToString(e.Key), aws.ToString(e.Message)))
				}
				return pathErr("removeAll", name, errors.Join(collected...))
			}
		}
		params.ContinuationToken = list.NextContinuationToken
		if params.ContinuationToken == nil {
			break
		}
	}
	return nil
}

// walkFiles returns an iterator that yields FileRefs for files in the dir.
// Each yielded FileRef carries fsys in its FS field, matching the contract
// of the local fileWalk path (fs/fs.go), so callers can open files through
// the ref without holding the backend separately.
func walkFiles(ctx context.Context, fsys ocflfs.FS, api FilesAPI, buck string, dir string) iter.Seq2[*ocflfs.FileRef, error] {
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
				// A non-AWS FilesAPI implementation may return partial
				// entries whose Key is nil. Such objects cannot be
				// addressed and must be skipped; dereferencing the nil
				// pointer below would panic inside the iterator instead
				// of returning an error.
				if s3obj.Key == nil {
					continue
				}
				// skip S3 directory placeholder objects: zero-byte keys ending
				// with "/" created by the S3 console or some clients to
				// represent directories. They are not files and would
				// otherwise appear as phantom empty files in OCFL inventories.
				// This also skips the directory prefix's own placeholder
				// (e.g. "dir/" when listing under prefix "dir/").
				if strings.HasSuffix(*s3obj.Key, "/") {
					continue
				}
				refPath := *s3obj.Key
				if dir != "." {
					refPath = strings.TrimPrefix(refPath, dir+"/")
				}
				info := &ocflfs.FileRef{
					FS:      fsys,
					BaseDir: dir,
					Path:    refPath,
					Info: &iofsInfo{
						name:    path.Base(*s3obj.Key),
						size:    aws.ToInt64(s3obj.Size),
						mode:    fileMode,
						modTime: aws.ToTime(s3obj.LastModified),
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
// s3File is safe for concurrent use. Two mutexes divide the mutable state:
//
//   - mu guards the body pointer and the generation counter. Seek and Close
//     take mu to close and discard the body and to move the file offset, and
//     every Read takes mu only to snapshot the current body, offset and
//     generation before touching the network.
//   - readMu serializes the network reads themselves: concurrent Reads must
//     not interleave on the same response body, so a Read holds readMu for
//     the whole body (re)creation and body.Read span. Seek and Close never
//     take readMu, which is what lets them return promptly while a Read is
//     blocked streaming a large object.
//
// The offset is an atomic.Int64 because it is written from two lock domains:
// Seek stores it under mu, while a completing Read adds its byte count
// without holding mu. The generation counter lets a Read that raced a Seek
// detect that its snapshot is stale: Seek bumps gen whenever it invalidates
// the body, and a Read whose snapshot generation no longer matches discards
// its byte count instead of applying it to the new position. A Read that
// races a Seek may therefore return data from the pre-seek position (or an
// error, if the Seek closed the body mid-read); the next Read observes the
// post-seek state.
type s3File struct {
	// mu guards body and gen. offset is atomic and readMu is independent.
	mu sync.Mutex

	// readMu serializes body (re)creation and body.Read calls.
	readMu sync.Mutex

	ctx    context.Context
	api    OpenFileAPI
	bucket string
	key    string
	body   io.ReadCloser
	info   *s3.HeadObjectOutput
	offset atomic.Int64 // current position in the file
	gen    int64        // bumped by Seek when the body is invalidated
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
	// Serialize the network access: at most one body may exist at a time and
	// body.Read calls must not interleave on the same response body. Seek
	// and Close do not take readMu, so they stay prompt while this Read is
	// blocked on the network.
	f.readMu.Lock()
	defer f.readMu.Unlock()

	for {
		// Snapshot the current body, offset and generation under mu. All
		// network I/O below happens without mu, so a concurrent Seek
		// returns as soon as it can acquire mu.
		f.mu.Lock()
		gen := f.gen
		off := f.offset.Load()
		size := *f.info.ContentLength
		body := f.body
		f.mu.Unlock()

		if off >= size {
			return 0, io.EOF
		}
		if body == nil {
			params := &s3.GetObjectInput{
				Bucket: &f.bucket,
				Key:    &f.key,
				// ensure unchanged since open
				IfMatch:           f.info.ETag,
				IfUnmodifiedSince: f.info.LastModified,
			}
			if off > 0 {
				rangeStr := fmt.Sprintf("bytes=%d-", off)
				params.Range = &rangeStr
			}
			obj, err := f.api.GetObject(f.ctx, params)
			if err != nil {
				return 0, err
			}
			// A Seek that landed while the object was being fetched has
			// invalidated this body's position: discard it and start over
			// from the new offset.
			f.mu.Lock()
			if f.gen != gen {
				f.mu.Unlock()
				obj.Body.Close()
				continue
			}
			f.body = obj.Body
			f.mu.Unlock()
			body = obj.Body
		}
		n, err := body.Read(p)
		// Apply the byte count only if the generation still matches the
		// snapshot: if a Seek invalidated the body mid-read, this data came
		// from the pre-seek position and must not move the new offset.
		if n > 0 {
			f.mu.Lock()
			if f.gen == gen {
				f.offset.Add(int64(n))
			}
			f.mu.Unlock()
		}
		return n, err
	}
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
// The body reader is closed and reset and the offset updated under f.mu, and
// the generation counter is bumped so in-flight Reads (which snapshot the
// body and offset without holding mu across their network read) can detect
// that their body is stale and drop their byte count instead of applying it
// to the new position. A Read that races a Seek may return data from the
// pre-seek position. Seek itself never blocks on a Read in progress: an
// in-flight Read holds only readMu, which Seek does not take, so Seek
// returns promptly even while a large object is being streamed.
func (f *s3File) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	size := *f.info.ContentLength
	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = f.offset.Load() + offset
	case io.SeekEnd:
		newOffset = size + offset
	default:
		return 0, errors.New("s3: invalid whence")
	}
	if newOffset < 0 {
		return 0, errors.New("s3: negative position")
	}
	// Position changed: close and discard any live body and bump the
	// generation counter so in-flight work that snapshotted the old
	// position discards its results — both a Read blocked in body.Read and
	// a Read whose GetObject is still in flight (no body has been installed
	// yet) are stale once the offset moves. The counter is bumped only when
	// the position actually changes: a no-op seek must not make in-flight
	// Reads drop their byte counts. A close error does not fail the Seek,
	// but it may mean a connection leaked, so log it to make it visible.
	cur := f.offset.Load()
	if newOffset != cur {
		if f.body != nil {
			if err := f.body.Close(); err != nil && f.logger != nil {
				f.logger.DebugContext(f.ctx, "s3:seek:close", "bucket", f.bucket, "key", f.key, "error", err)
			}
			f.body = nil
		}
		f.gen++
		f.offset.Store(newOffset)
	}
	return newOffset, nil
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

// notExistErr wraps fs.ErrNotExist around err instead of replacing it, so
// callers can match the missing-file contract with errors.Is(err,
// fs.ErrNotExist) while the original smithy/HTTP error (status code, request
// ID, etc.) remains reachable through the unwrap chain for debugging against
// MinIO and real S3. All not-exist error paths in this package must use it.
func notExistErr(err error) error {
	return fmt.Errorf("%w: %w", fs.ErrNotExist, err)
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

// errIsNotExist reports whether err represents a missing object ("not found")
// error from S3 or an S3-compatible store. Callers use it to map HeadObject
// (and similar) failures to fs.ErrNotExist.
//
// The error shapes the AWS SDK v2 can produce for a missing object depend on
// the service and on whether the error response had a body:
//
//   - Operations whose errors deserialize from a body (e.g. GetObject) return
//     the typed shapes types.NotFound and types.NoSuchKey.
//   - HeadObject against real S3 returns *smithyhttp.ResponseError with HTTP
//     status 404 wrapping the failed error deserialization, because HEAD
//     responses carry no body from which to deserialize an error shape.
//   - Some S3-compatible stores (e.g. MinIO) return a HEAD 404 with an XML
//     body whose code (commonly "NoSuchKey") is not one of the shapes
//     HeadObject's deserializer recognizes (it only maps "NotFound"); in that
//     case the SDK falls back to *smithy.GenericAPIError carrying the code.
func errIsNotExist(err error) bool {
	var notFoundErr *types.NotFound
	if errors.As(err, &notFoundErr) {
		return true
	}
	var noKeyErr *types.NoSuchKey
	if errors.As(err, &noKeyErr) {
		return true
	}
	// HeadObject on a missing object: real S3 returns an HTTP 404 with no
	// body, which surfaces as a generic smithy http response error.
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) && respErr.Response != nil &&
		respErr.Response.StatusCode == http.StatusNotFound {
		return true
	}
	// S3-compatible stores that include an error body on HEAD 404 (e.g. MinIO
	// with code "NoSuchKey", which HeadObject's deserializer does not map to a
	// typed shape) surface as a generic API error carrying the code.
	var genericErr *smithy.GenericAPIError
	if errors.As(err, &genericErr) {
		switch genericErr.Code {
		case "NotFound", "NoSuchKey":
			return true
		}
	}
	return false
}
