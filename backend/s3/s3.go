package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"slices"
	"time"

	// awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	// awsv2cfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	s3mgr "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"golang.org/x/sync/errgroup"
	// "github.com/aws/smithy-go"
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
)

var (
	delim         = "/"
	maxKeys int32 = 1000
)

// FS is an implementation of ocfl.FS, ocfl.WriteFS, and ocfl.CopyFS for an
// s3 bucket.
type FS struct {
	Client *s3v2.Client
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

// OpenFile impleme
func (b *FS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, pathErr("open", name, fs.ErrInvalid)
	}
	params := &s3v2.GetObjectInput{
		Bucket: &b.Bucket,
		Key:    &name,
	}
	obj, err := b.Client.GetObject(ctx, params)
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
	return &s3File{bucket: b.Bucket, key: name, obj: obj}, nil
}

func (b *FS) ReadDir(ctx context.Context, dir string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(dir) {
		return nil, pathErr("readdir", dir, fs.ErrInvalid)
	}
	params := &s3v2.ListObjectsV2Input{
		Bucket:    &b.Bucket,
		Delimiter: &delim,
		MaxKeys:   &maxKeys,
	}
	if dir != "." {
		params.Prefix = aws.String(dir + "/")
	}
	entries := make([]fs.DirEntry, 0, 32)
	for {
		list, err := b.Client.ListObjectsV2(ctx, params)
		if err != nil {
			return nil, pathErr("readdir", dir, err)
		}
		numDirs := len(list.CommonPrefixes)
		numFiles := len(list.Contents)
		newEntries := make([]fs.DirEntry, numDirs+numFiles)
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
		entries = append(entries, newEntries...)
		params.ContinuationToken = list.NextContinuationToken
		if params.ContinuationToken == nil {
			break
		}
	}
	sortEntries := func(a, b fs.DirEntry) int {
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
	slices.SortFunc(entries, sortEntries)
	return entries, nil
}

func (b *FS) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	if !fs.ValidPath(name) || name == "." {
		return 0, pathErr("write", name, fs.ErrInvalid)
	}
	var (
		totalSize   = int64(-1)
		concurrency = b.UploadConcurrency
		partSize    = b.DefaultUploadPartSize
		numParts    = maxParts
	)
	if concurrency == 0 {
		concurrency = defaultUploadConcurrency
	}
	if partSize == 0 || partSize < minPartSize {
		partSize = defaultUploadPartSize
	}
	// try to guess reader size: value used
	// to adjust partSize
	switch r := r.(type) {
	case *io.LimitedReader:
		totalSize = r.N
	case fs.File:
		if info, err := r.Stat(); err == nil {
			totalSize = info.Size()
		}
	}
	if totalSize > 0 {
		partSize, numParts = adjustPartSize(totalSize, partSize, numParts)
	}
	uploader := s3mgr.NewUploader(b.Client, func(u *s3mgr.Uploader) {
		u.Concurrency = concurrency
		u.PartSize = partSize
		u.MaxUploadParts = numParts
	})
	countReader := &countReader{Reader: r}
	params := &s3v2.PutObjectInput{
		Bucket: &b.Bucket,
		Key:    &name,
		Body:   countReader,
	}
	_, err := uploader.Upload(ctx, params)
	if err != nil {
		return 0, &fs.PathError{Op: "write", Path: name, Err: err}
	}
	return countReader.size, nil
}

func (b *FS) Remove(ctx context.Context, name string) error {
	if !fs.ValidPath(name) {
		return pathErr("remove", name, fs.ErrInvalid)
	}
	if name == "." {
		return pathErr("remove", name, fs.ErrNotExist)
	}
	_, err := b.Client.DeleteObject(ctx, &s3v2.DeleteObjectInput{
		Bucket: &b.Bucket,
		Key:    aws.String(name),
	})
	if err != nil {
		return pathErr("remove", name, err)
	}
	return nil
}

func (b *FS) RemoveAll(ctx context.Context, name string) error {
	if !fs.ValidPath(name) {
		return pathErr("removeall", name, fs.ErrInvalid)
	}
	params := &s3v2.ListObjectsV2Input{
		Bucket:  &b.Bucket,
		MaxKeys: &maxKeys,
	}
	if name != "." {
		params.Prefix = aws.String(name + "/")
	}
	for {
		list, err := b.Client.ListObjectsV2(ctx, params)
		if err != nil {
			return pathErr("removeall", name, err)
		}
		for _, obj := range list.Contents {
			_, err := b.Client.DeleteObject(ctx, &s3v2.DeleteObjectInput{
				Bucket: &b.Bucket,
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

func (b *FS) Copy(ctx context.Context, dst, src string) error {
	if !fs.ValidPath(src) || src == "." {
		return pathErr("copy", src, fs.ErrInvalid)
	}
	if !fs.ValidPath(dst) || dst == "." {
		return pathErr("copy", dst, fs.ErrInvalid)
	}
	escapedSrc := url.QueryEscape(b.Bucket + "/" + src)
	params := &s3v2.CopyObjectInput{
		Bucket:     &b.Bucket,
		CopySource: &escapedSrc,
		Key:        &dst,
	}
	_, err := b.Client.CopyObject(ctx, params)
	if err != nil {
		// TODO: if copy fails because the src is > 5GB,
		// fall back to multipart copy
		return pathErr("copy", src, err)
	}
	return nil
}

func (b *FS) MultipartCopy(ctx context.Context, dst, src string) (err error) {
	headParams := &s3v2.HeadObjectInput{
		Bucket: &b.Bucket,
		Key:    &src,
	}
	srcObj, err := b.Client.HeadObject(ctx, headParams)
	if err != nil {
		err = pathErr("copy", src, err)
		return
	}
	if srcObj.ContentLength == nil {
		err = pathErr("copy", src, errors.New("missing content length"))
		return
	}
	srcSize := *srcObj.ContentLength
	partSize := b.DefaultCopyPartSize
	if partSize == 0 || partSize < minPartSize {
		partSize = defaultCopyPartSize
	}
	partSize, partCount := adjustPartSize(srcSize, partSize, maxParts)
	completedParts := make([]types.CompletedPart, partCount)
	uploadParams := &s3v2.CreateMultipartUploadInput{
		Bucket: &b.Bucket,
		Key:    &dst,
	}
	newUp, err := b.Client.CreateMultipartUpload(ctx, uploadParams)
	if err != nil {
		err = pathErr("copy", dst, err)
		return
	}
	defer func() {
		// cleanup or complete the multipart upload
		switch {
		case err != nil:
			params := &s3v2.AbortMultipartUploadInput{
				Bucket:   &b.Bucket,
				Key:      &dst,
				UploadId: newUp.UploadId,
			}
			_, abortErr := b.Client.AbortMultipartUpload(ctx, params)
			err = errors.Join(err, abortErr)
		default:
			upload := &types.CompletedMultipartUpload{
				Parts: completedParts,
			}
			params := &s3v2.CompleteMultipartUploadInput{
				Bucket:          &b.Bucket,
				Key:             &dst,
				UploadId:        newUp.UploadId,
				MultipartUpload: upload,
			}
			_, err = b.Client.CompleteMultipartUpload(ctx, params)
		}
	}()
	grp, grpCtx := errgroup.WithContext(ctx)
	grpLimit := b.CopyPartConcurrency
	if grpLimit == 0 {
		grpLimit = defaultCopyPartConcurrency
	}
	grp.SetLimit(grpLimit)
	copySource := url.QueryEscape(b.Bucket + "/" + src)
	for i := int32(0); i < partCount; i++ {
		i := i
		grp.Go(func() error {
			var err error
			partNum := i + 1
			srcRange := byteRange(partNum, partSize, srcSize)
			params := &s3v2.UploadPartCopyInput{
				Bucket:          &b.Bucket,
				CopySource:      &copySource,
				Key:             &dst,
				UploadId:        newUp.UploadId,
				PartNumber:      &partNum,
				CopySourceRange: &srcRange,
			}
			result, err := b.Client.UploadPartCopy(grpCtx, params)
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
