package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"net/http"
	"path"
	"slices"
	"strings"
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

	// modes retured by Stat()
	fileMode = 0644 | fs.ModeIrregular
	dirMode  = 0755 | fs.ModeDir
)

// maxCopySize is the largest source CopyObject accepts. It is a fixed S3 API
// limit, not a tunable: a source larger than this has to be copied part by
// part, which is what MultiCopier does. copy compares the source's HEAD
// ContentLength against it rather than issuing a CopyObject to find out, so a
// large copy costs no doomed request and the choice does not depend on how a
// store words its error. A store that enforces a smaller limit than this one
// fails rather than falling back -- see [BucketFS.Copy].
const maxCopySize int64 = 5 * 1024 * megabyte

// maxDeleteBatch is the most keys DeleteObjects accepts in one request. It is
// a fixed S3 API limit, not a tunable: a larger request is rejected outright.
// removeAll splits each listing page into batches of this size, so the page
// size below can change without silently sending an oversized delete.
const maxDeleteBatch = 1000

var (
	// these are variable because we need pass them as pointers
	delim = "/"
	// maxKeys is the page size for every listing in this package. It is the
	// batch limit today, which makes a page exactly one delete request.
	maxKeys int32 = maxDeleteBatch
)

// Compile-time check that s3File implements io.Seeker
var _ io.Seeker = (*s3File)(nil)

func openFile(ctx context.Context, api OpenFileAPI, buck string, name string) (fs.File, error) {
	if !fs.ValidPath(name) || name == "." {
		return nil, pathErr("open", name, fs.ErrInvalid)
	}
	headIn := &s3.HeadObjectInput{Bucket: &buck, Key: &name}
	headOut, err := api.HeadObject(ctx, headIn)
	if err != nil {
		if errIsNotExist(err) {
			err = notExistErr(err)
		}
		return nil, pathErr("open", name, err)
	}
	f := &s3File{
		ctx:    ctx,
		api:    api,
		bucket: buck,
		key:    name,
		info:   headOut,
	}
	return f, nil
}

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
				if !prefixHasContent {
					// treat prefix without objects as a missing directory
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
	// ContentLength is left unset. The uploader sends its own buffered chunks,
	// and the SDK sets each request's Content-Length from the bytes it is
	// actually sending, so a value set here overrides that rather than
	// enabling it. An explicit one from an option is passed through as given.
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
		if errIsNotExist(err) {
			err = notExistErr(err)
		}
		return 0, pathErr("copy", src, err)
	}
	// The size decides the strategy, and the HEAD above already carries it.
	// A store may omit ContentLength on a HEAD response; refusing here is
	// better than carrying an unknown size into a choice that depends on it.
	if srcHead.ContentLength == nil {
		return 0, pathErr("copy", src, errors.New("missing content length"))
	}
	srcSize := *srcHead.ContentLength
	if srcSize > maxCopySize {
		// too large for CopyObject: copy it part by part instead. The HEAD is
		// handed on so MultiCopier does not repeat it.
		return NewMultiCopier(api, opts...).Copy(ctx, buck, dst, src, srcHead)
	}
	escapedSrc := encodeCopySource(buck, src)
	params := &s3.CopyObjectInput{
		Bucket:     &buck,
		CopySource: &escapedSrc, // percent-encoded per encodeCopySource
		Key:        &dst,
	}
	if _, err := api.CopyObject(ctx, params); err != nil {
		return 0, pathErr("copy", src, err)
	}
	return srcSize, nil
}

func remove(ctx context.Context, api RemoveAPI, b string, name string) error {
	if !fs.ValidPath(name) {
		return pathErr("remove", name, fs.ErrInvalid)
	}
	// "." names the bucket, not an object, so removing it is a bad name
	// rather than a failed removal — matching openFile and write, which
	// already reject "." that way. The guard issues no request.
	if name == "." {
		return pathErr("remove", name, fs.ErrInvalid)
	}
	// DeleteObject is idempotent: it answers 204 whether or not the key was
	// there, and says nothing about which it was. The WriteFS.Remove
	// contract requires fs.ErrNotExist for a name that is not there, so the
	// existence has to be established separately, with a HEAD.
	//
	// Two consequences, documented rather than engineered around. The probe
	// is not atomic with the delete: a key another writer removes between
	// the HEAD and the DELETE is reported as removed, since the DELETE
	// succeeds either way. And Remove costs two round trips instead of one.
	// The library calls Remove only to undo a failed update and to replace
	// the storage root's layout file, so the extra HEAD lands off the happy
	// path; a caller that removes keys in bulk should use RemoveAll, which
	// still lists and deletes without probing.
	if _, err := api.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &b,
		Key:    aws.String(name),
	}); err != nil {
		if errIsNotExist(err) {
			err = notExistErr(err)
		}
		return pathErr("remove", name, err)
	}
	_, err := api.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &b,
		Key:    aws.String(name),
	})
	if err != nil {
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
		// The trailing separator is what keeps "a" from matching the
		// sibling key "ab". "." carries no prefix at all: it names the
		// whole bucket.
		params.Prefix = aws.String(name + "/")
	}
	// Deletion is best-effort, per the WriteFS.RemoveAll contract: one key
	// that will not delete must not abandon the keys after it, which would
	// leave a partial deletion and report only the first of possibly many
	// failures. That holds across pages too -- a page that reports failures
	// must still be followed by the next one.
	var errs error
	for {
		list, err := api.ListObjectsV2(ctx, params)
		if err != nil {
			// Without a listing there is nothing further to attempt, so
			// this one does stop the loop.
			return errors.Join(errs, pathErr("removeall", name, err))
		}
		errs = errors.Join(errs, deleteKeys(ctx, api, buck, name, list.Contents))
		params.ContinuationToken = list.NextContinuationToken
		if params.ContinuationToken == nil {
			break
		}
	}
	return errs
}

// deleteKeys deletes one listing page in as few requests as the API allows,
// and returns every per-key failure it can see, joined.
//
// name is the name the caller passed RemoveAll; it is used only for a failure
// that says nothing about individual keys.
func deleteKeys(ctx context.Context, api RemoveAllAPI, buck, name string, contents []types.Object) error {
	ids := make([]types.ObjectIdentifier, 0, len(contents))
	for _, obj := range contents {
		if obj.Key == nil {
			continue
		}
		ids = append(ids, types.ObjectIdentifier{Key: obj.Key})
	}
	var errs error
	for batch := range slices.Chunk(ids, maxDeleteBatch) {
		out, err := api.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: &buck,
			Delete: &types.Delete{
				Objects: batch,
				// In quiet mode the response carries only the keys that
				// failed, which is all this code reads.
				Quiet: aws.Bool(true),
			},
		})
		if err != nil {
			// A transport-level failure says nothing about individual keys:
			// every key in the batch is simply unaccounted for. Report it
			// once against the name the caller passed rather than repeating
			// it for up to maxDeleteBatch keys, and keep going -- the next
			// batch and the next page may well succeed.
			errs = errors.Join(errs, pathErr("removeall", name, err))
			continue
		}
		// DeleteObjects answers 200 even when individual keys fail, listing
		// them in the response body. Checking only the transport error would
		// report a successful RemoveAll for a prefix that is still partly
		// populated -- worse than the per-key loop this replaced, which at
		// least saw each failure.
		if out == nil {
			continue
		}
		for _, e := range out.Errors {
			errs = errors.Join(errs, pathErr("removeall", aws.ToString(e.Key), deleteErr(e)))
		}
	}
	return errs
}

// deleteErr turns one entry of a DeleteObjects response's Errors list into an
// error. Neither field is guaranteed to be set, so the fallbacks matter: a
// failure reported with no reason at all still has to read as a failure.
func deleteErr(e types.Error) error {
	code, msg := aws.ToString(e.Code), aws.ToString(e.Message)
	switch {
	case code != "" && msg != "":
		return fmt.Errorf("%s: %s", code, msg)
	case code != "":
		return errors.New(code)
	case msg != "":
		return errors.New(msg)
	}
	return errors.New("delete failed, no reason reported")
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

// s3File implements fs.File and io.Seeker
type s3File struct {
	ctx    context.Context
	api    OpenFileAPI
	bucket string
	key    string
	body   io.ReadCloser
	info   *s3.HeadObjectOutput
	offset int64 // current position in the file
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
// Seeking invalidates any existing body reader, causing the next Read to
// issue a new GetObject request with the appropriate Range header.
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
	// Close existing body if position changed
	if f.body != nil && newOffset != f.offset {
		f.body.Close()
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

// countReader is a reader that updates a size counter with each read. It is
// what write returns a byte count from. It must not grow an io.ReaderAt: the
// upload manager has a fast path for one that reads each part from absolute
// offset zero rather than from where the reader is.
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

// errIsNotExist reports whether err is an S3 API error meaning "that object
// is not there". Four shapes reach here, because what comes back depends on
// the operation and on the store:
//
//   - *types.NoSuchKey, deserialized from the response body of GetObject,
//     CopyObject and UploadPartCopy;
//   - *types.NotFound, which is what HeadObject produces: a HEAD response
//     has no body to deserialize a code from, so the SDK derives one from
//     the 404 status;
//   - *smithy.GenericAPIError carrying the code as a string, from an
//     S3-compatible store that sent a code the SDK could not resolve to one
//     of the typed errors;
//   - any error carrying a 404 response, for a store whose code neither the
//     SDK nor the cases above recognize.
//
// A missing bucket is deliberately not in that list even though it is also a
// 404. It is a configuration error, not a missing file: a caller told
// fs.ErrNotExist would conclude the object was never written, when in fact
// nothing at all can be read or written.
func errIsNotExist(err error) bool {
	var notFoundErr *types.NotFound
	if errors.As(err, &notFoundErr) {
		return true
	}
	var noKeyErr *types.NoSuchKey
	if errors.As(err, &noKeyErr) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound":
			return true
		case "NoSuchBucket":
			return false
		}
	}
	var respErr *smithyhttp.ResponseError
	return errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusNotFound
}

// notExistErr wraps err so that errors.Is(err, fs.ErrNotExist) matches while
// the cause stays reachable. Replacing the cause with fs.ErrNotExist -- what
// this package used to do -- throws away the status code, the request ID and
// the API error code, which is most of what makes a failure against a real
// endpoint diagnosable.
func notExistErr(err error) error {
	return fmt.Errorf("%w: %w", fs.ErrNotExist, err)
}
