package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"net/url"
	"path"
	"slices"
	"strings"
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

	// error message returned when copy fails because source is too large: used
	// to trigger multipart upload. (This appears to be the only way to check
	// this error).
	copySrcTooLarge = "copy source is larger than the maximum allowable size"

	// modes retured by Stat()
	fileMode = 0644 | fs.ModeIrregular
	dirMode  = 0755 | fs.ModeDir
)

var (
	// these are variable because we need pass them as pointers
	delim         = "/"
	maxKeys int32 = 1000
)

func openFile(ctx context.Context, api OpenFileAPI, buck string, name string) (fs.File, error) {
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
	if _, err := api.CopyObject(ctx, params); err != nil {
		// if the source is too large, try multipart copy.
		// this error doesn't seem to have a specific type
		// associated with it.
		if strings.Contains(err.Error(), copySrcTooLarge) {
			// source is too large for basic copy -- try multipart copy
			return NewMultiCopier(api, opts...).Copy(ctx, buck, dst, src, srcHead)
		}
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
		params.Prefix = aws.String(name + "/")
	}
	for {
		list, err := api.ListObjectsV2(ctx, params)
		if err != nil {
			return pathErr("removeall", name, err)
		}
		for _, obj := range list.Contents {
			_, err := api.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: &buck,
				Key:    obj.Key,
			})
			if err != nil {
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

// s3File implements fs.File
type s3File struct {
	ctx    context.Context
	api    OpenFileAPI
	bucket string
	key    string
	body   io.ReadCloser
	info   *s3.HeadObjectOutput
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
	if f.body == nil {
		params := &s3.GetObjectInput{Bucket: &f.bucket, Key: &f.key}
		obj, err := f.api.GetObject(f.ctx, params)
		if err != nil {
			return 0, err
		}
		f.body = obj.Body
	}
	return f.body.Read(p)
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
