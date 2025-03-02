package s3

import (
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
	s3mgr "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"golang.org/x/sync/errgroup"
)

const (
	megabyte int64 = 1024 * 1024
	gigabyte int64 = 1024 * megabyte

	minPartSize = s3mgr.MinUploadPartSize
	maxParts    = s3mgr.MaxUploadParts

	defaultUploadPartSize    = s3mgr.DefaultUploadPartSize
	defaultUploadConcurrency = s3mgr.DefaultUploadConcurrency

	defaultCopyPartConcurrency = 12
	defaultCopyPartSize        = 64 * megabyte
	partSizeIncrement          = 1 * megabyte

	copySrcTooLarge = "copy source is larger than the maximum allowable size"
)

var (
	delim         = "/"
	maxKeys int32 = 1000
)

func openFile(ctx context.Context, api OpenFileAPI, buck string, name string) (fs.File, error) {
	if !fs.ValidPath(name) || name == "." {
		return nil, pathErr("open", name, fs.ErrInvalid)
	}
	params := &s3v2.GetObjectInput{Bucket: &buck, Key: &name}
	obj, err := api.GetObject(ctx, params)
	if err != nil {
		fsErr := &fs.PathError{Op: "open", Path: name}
		var awsErr *types.NoSuchKey
		switch {
		case errors.As(err, &awsErr):
			fsErr.Err = fs.ErrNotExist
		default:
			fsErr.Err = err
		}
		return nil, fsErr
	}
	return &s3File{bucket: buck, key: name, obj: obj}, nil
}

func readDir(ctx context.Context, api ReadDirAPI, buck string, dir string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(dir) {
		return nil, pathErr("readdir", dir, fs.ErrInvalid)
	}
	params := &s3v2.ListObjectsV2Input{
		Bucket:    &buck,
		Delimiter: &delim,
		MaxKeys:   &maxKeys,
	}
	if dir != "." {
		params.Prefix = aws.String(dir + "/")
	}
	entries := make([]fs.DirEntry, 0, 32)
	for {
		list, err := api.ListObjectsV2(ctx, params)
		if err != nil {
			return nil, pathErr("readdir", dir, err)
		}
		numDirs := len(list.CommonPrefixes)
		numFiles := len(list.Contents)
		numEntries := numDirs + numFiles
		if numEntries == 0 {
			break
		}
		newEntries := make([]fs.DirEntry, numEntries)
		for i, item := range list.CommonPrefixes {
			newEntries[i] = &iofsInfo{
				name: path.Base(*item.Prefix),
				mode: fs.ModeDir,
			}
		}
		for i, item := range list.Contents {
			newEntries[numDirs+i] = &iofsInfo{
				name:    path.Base(*item.Key),
				size:    *item.Size,
				mode:    fs.ModeIrregular,
				modTime: *item.LastModified,
				// sys:     &item,
			}
		}
		slices.SortFunc(newEntries, cmpDirEntries)
		entries = append(entries, newEntries...)
		params.ContinuationToken = list.NextContinuationToken
		if params.ContinuationToken == nil {
			break
		}
	}
	if len(entries) == 0 {
		// treat the prefix as a non-existing directory
		return nil, pathErr("readdir", dir, fs.ErrNotExist)
	}
	return entries, nil
}

func write(ctx context.Context, api WriteAPI, buck string, key string, r io.Reader, size int64, psize int64, conc int) (int64, error) {
	if !fs.ValidPath(key) || key == "." {
		return 0, pathErr("write", key, fs.ErrInvalid)
	}
	numParts := maxParts
	if conc < 1 {
		conc = defaultUploadConcurrency
	}
	if psize < minPartSize {
		psize = defaultUploadPartSize
	}
	if size > 0 {
		psize, numParts = adjustPartSize(size, psize, numParts)
	}
	uploader := s3mgr.NewUploader(api, func(u *s3mgr.Uploader) {
		u.Concurrency = conc
		u.PartSize = psize
		u.MaxUploadParts = numParts
	})
	countReader := &countReader{Reader: r}
	params := &s3v2.PutObjectInput{
		Bucket: &buck,
		Key:    &key,
		Body:   countReader,
	}
	if size > 0 {
		params.ContentLength = &size
	}
	_, err := uploader.Upload(ctx, params)
	if err != nil {
		return 0, &fs.PathError{Op: "write", Path: key, Err: err}
	}
	return countReader.size, nil
}

func copy(ctx context.Context, api CopyAPI, buck string, dst, src string, psize int64, conc int) error {
	if !fs.ValidPath(src) || src == "." {
		return pathErr("copy", src, fs.ErrInvalid)
	}
	if !fs.ValidPath(dst) || dst == "." {
		return pathErr("copy", dst, fs.ErrInvalid)
	}
	escapedSrc := url.QueryEscape(buck + "/" + src)
	params := &s3v2.CopyObjectInput{
		Bucket:     &buck,
		CopySource: &escapedSrc,
		Key:        &dst,
	}
	_, err := api.CopyObject(ctx, params)
	if err != nil {
		// if the source is too large, try multipart copy.
		// this error doesn't seem to have a specific type
		// associated with it.
		if strings.Contains(err.Error(), copySrcTooLarge) {
			return multipartCopy(ctx, api, buck, dst, src, psize, conc)
		}
		return pathErr("copy", src, err)
	}
	return nil
}

func multipartCopy(ctx context.Context, api CopyAPI, buck string, dst, src string, psize int64, conc int) (err error) {
	headParams := &s3v2.HeadObjectInput{Bucket: &buck, Key: &src}
	srcObj, err := api.HeadObject(ctx, headParams)
	if err != nil {
		err = pathErr("copy", src, err)
		return
	}
	if srcObj.ContentLength == nil {
		err = pathErr("copy", src, errors.New("missing content length"))
		return
	}
	srcSize := *srcObj.ContentLength
	if psize < minPartSize {
		psize = defaultCopyPartSize
	}
	if conc < 1 {
		conc = defaultCopyPartConcurrency
	}
	psize, partCount := adjustPartSize(srcSize, psize, maxParts)
	completedParts := make([]types.CompletedPart, partCount)
	uploadParams := &s3v2.CreateMultipartUploadInput{Bucket: &buck, Key: &dst}
	newUp, err := api.CreateMultipartUpload(ctx, uploadParams)
	if err != nil {
		err = pathErr("copy", dst, err)
		return
	}
	defer func() {
		// complete or abort the multipart upload
		switch {
		case err != nil:
			params := &s3v2.AbortMultipartUploadInput{
				Bucket:   &buck,
				Key:      &dst,
				UploadId: newUp.UploadId,
			}
			_, abortErr := api.AbortMultipartUpload(ctx, params)
			err = errors.Join(err, abortErr)
		default:
			upload := &types.CompletedMultipartUpload{
				Parts: completedParts,
			}
			params := &s3v2.CompleteMultipartUploadInput{
				Bucket:          &buck,
				Key:             &dst,
				UploadId:        newUp.UploadId,
				MultipartUpload: upload,
			}
			_, err = api.CompleteMultipartUpload(ctx, params)
		}
	}()
	grp, grpCtx := errgroup.WithContext(ctx)
	grp.SetLimit(conc)
	copySource := url.QueryEscape(buck + "/" + src)
	for i := int32(0); i < partCount; i++ {
		i := i
		grp.Go(func() error {
			var err error
			partNum := i + 1
			srcRange := byteRange(partNum, psize, srcSize)
			params := &s3v2.UploadPartCopyInput{
				Bucket:          &buck,
				CopySource:      &copySource,
				Key:             &dst,
				UploadId:        newUp.UploadId,
				PartNumber:      &partNum,
				CopySourceRange: &srcRange,
			}
			result, err := api.UploadPartCopy(grpCtx, params)
			if err != nil {
				return err
			}
			completedParts[i] = types.CompletedPart{
				PartNumber: &partNum,
				ETag:       result.CopyPartResult.ETag,
			}
			return nil
		})
	}
	err = grp.Wait()
	return
}

func remove(ctx context.Context, api RemoveAPI, b string, name string) error {
	if !fs.ValidPath(name) {
		return pathErr("remove", name, fs.ErrInvalid)
	}
	if name == "." {
		return pathErr("remove", name, fs.ErrNotExist)
	}
	_, err := api.DeleteObject(ctx, &s3v2.DeleteObjectInput{
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
	params := &s3v2.ListObjectsV2Input{Bucket: &buck, MaxKeys: &maxKeys}
	if name != "." {
		params.Prefix = aws.String(name + "/")
	}
	for {
		list, err := api.ListObjectsV2(ctx, params)
		if err != nil {
			return pathErr("removeall", name, err)
		}
		for _, obj := range list.Contents {
			_, err := api.DeleteObject(ctx, &s3v2.DeleteObjectInput{
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
		params := &s3v2.ListObjectsV2Input{
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
						mode:    fs.ModeIrregular,
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
	bucket string
	key    string
	obj    *s3v2.GetObjectOutput
}

func (f *s3File) Stat() (fs.FileInfo, error) {
	return &iofsInfo{
		name:    path.Base(f.key),
		size:    *f.obj.ContentLength,
		mode:    fs.ModeIrregular,
		modTime: *f.obj.LastModified,
		sys:     f.obj,
	}, nil
}

func (f *s3File) Read(p []byte) (int, error) {
	return f.obj.Body.Read(p)
}

func (f *s3File) Close() error {
	return f.obj.Body.Close()
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

func adjustPartSize(total, defaultPartSize int64, maxParts int32) (psize int64, pcount int32) {
	psize = defaultPartSize
	for {
		pcount = int32(total / psize)
		if pcount < maxParts {
			break
		}
		psize += partSizeIncrement
	}
	if total%psize > 0 {
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

func cmpDirEntries(a, b fs.DirEntry) int {
	aN, bN := a.Name(), b.Name()
	switch {
	case aN < bN:
		return -1
	case aN > bN:
		return 1
	default:
		return 0
	}
}
