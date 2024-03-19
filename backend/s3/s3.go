package s3

import (
	"context"
	"errors"
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

var (
	delim         = "/"
	maxKeys int32 = 1000
)

func New(cfg aws.Config, bucket string) *S3Backend {
	client := s3v2.NewFromConfig(cfg)
	return &S3Backend{
		bucket: bucket,
		client: client,
	}
}

type S3Backend struct {
	bucket                    string
	client                    *s3v2.Client
	UploadConcurrency         int
	UploadPartCopyConcurrency int
	MaxUploadParts            int32
	DefaultUploadPartSize     int64
}

func (b *S3Backend) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, pathErr("open", name, fs.ErrInvalid)
	}
	params := &s3v2.GetObjectInput{
		Bucket: &b.bucket,
		Key:    &name,
	}
	obj, err := b.client.GetObject(ctx, params)
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
	return &s3File{bucket: b.bucket, key: name, obj: obj}, nil
}

func (b *S3Backend) ReadDir(ctx context.Context, dir string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(dir) {
		return nil, pathErr("readdir", dir, fs.ErrInvalid)
	}
	params := &s3v2.ListObjectsV2Input{
		Bucket:    &b.bucket,
		Delimiter: &delim,
		MaxKeys:   &maxKeys,
	}
	if dir != "." {
		params.Prefix = aws.String(dir + "/")
	}
	entries := make([]fs.DirEntry, 0, 32)
	for {
		list, err := b.client.ListObjectsV2(ctx, params)
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

func (b *S3Backend) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	if !fs.ValidPath(name) || name == "." {
		return 0, pathErr("write", name, fs.ErrInvalid)
	}
	var (
		totalSize   = int64(-1)
		concurrency = b.UploadConcurrency
		partSize    = b.DefaultUploadPartSize
		maxParts    = b.MaxUploadParts
	)
	if concurrency == 0 {
		concurrency = s3mgr.DefaultUploadConcurrency
	}
	if partSize == 0 {
		partSize = s3mgr.DefaultUploadPartSize
	}
	if maxParts == 0 {
		maxParts = s3mgr.MaxUploadParts
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
	if totalSize > (int64(maxParts) * partSize) {
		// this logic is taken from aws v2 sdk.
		partSize = (totalSize / int64(maxParts)) + 1
	}
	uploader := s3mgr.NewUploader(b.client, func(u *s3mgr.Uploader) {
		u.Concurrency = concurrency
		u.PartSize = partSize
		u.MaxUploadParts = maxParts
	})
	countReader := &countReader{Reader: r}
	params := &s3v2.PutObjectInput{
		Bucket: &b.bucket,
		Key:    &name,
		Body:   countReader,
		// checksums?
	}
	_, err := uploader.Upload(ctx, params)
	if err != nil {
		return 0, &fs.PathError{Op: "write", Path: name, Err: err}
	}
	return countReader.size, nil
}

func (b *S3Backend) Remove(ctx context.Context, name string) error {
	if !fs.ValidPath(name) {
		return pathErr("remove", name, fs.ErrInvalid)
	}
	if name == "." {
		return pathErr("remove", name, fs.ErrNotExist)
	}
	_, err := b.client.DeleteObject(ctx, &s3v2.DeleteObjectInput{
		Bucket: &b.bucket,
		Key:    aws.String(name),
	})
	if err != nil {
		return pathErr("remove", name, err)
	}
	return nil
}

func (b *S3Backend) RemoveAll(ctx context.Context, name string) error {
	if !fs.ValidPath(name) {
		return pathErr("removeall", name, fs.ErrInvalid)
	}
	params := &s3v2.ListObjectsV2Input{
		Bucket:  &b.bucket,
		MaxKeys: &maxKeys,
	}
	if name != "." {
		params.Prefix = aws.String(name + "/")
	}
	for {
		list, err := b.client.ListObjectsV2(ctx, params)
		if err != nil {
			return pathErr("removeall", name, err)
		}
		for _, obj := range list.Contents {
			_, err := b.client.DeleteObject(ctx, &s3v2.DeleteObjectInput{
				Bucket: &b.bucket,
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

func (b *S3Backend) Copy(ctx context.Context, dst, src string) error {
	if !fs.ValidPath(src) || src == "." {
		return pathErr("copy", src, fs.ErrInvalid)
	}
	if !fs.ValidPath(dst) || dst == "." {
		return pathErr("copy", dst, fs.ErrInvalid)
	}
	escapedSrc := url.QueryEscape(b.bucket + "/" + src)
	params := &s3v2.CopyObjectInput{
		Bucket:     &b.bucket,
		CopySource: &escapedSrc,
		Key:        &dst,
	}
	_, err := b.client.CopyObject(ctx, params)
	if err != nil {
		// TODO: if copy fails because the src is > 5GB,
		// fall back to multipart copy
		return pathErr("copy", src, err)
	}

	return nil
}

func (b *S3Backend) multipartCopy(ctx context.Context, dst, src string) error {
	headParams := &s3v2.HeadObjectInput{
		Bucket:     &b.bucket,
		Key:        &src,
		PartNumber: aws.Int32(10_000),
	}
	srcObj, err := b.client.HeadObject(ctx, headParams)
	if err != nil {
		return pathErr("copy", src, err)
	}
	var partCount int32 = 1
	if srcObj.PartsCount != nil {
		partCount = *srcObj.PartsCount
	}
	uploadParams := &s3v2.CreateMultipartUploadInput{
		Bucket: &b.bucket,
		Key:    &dst,
	}
	newUp, err := b.client.CreateMultipartUpload(ctx, uploadParams)
	if err != nil {
		return pathErr("copy", dst, err)
	}
	completedParts := make([]types.CompletedPart, partCount)
	escapedCopySource := url.QueryEscape(src)
	grp, grpCtx := errgroup.WithContext(ctx)
	grp.SetLimit(b.UploadPartCopyConcurrency)
	for i := int32(0); i < partCount; i++ {
		i := i
		grp.Go(func() error {
			partNum := aws.Int32(i)
			params := &s3v2.UploadPartCopyInput{
				Bucket:     &b.bucket,
				CopySource: &escapedCopySource,
				Key:        &dst,
				UploadId:   newUp.UploadId,
				PartNumber: partNum,
			}
			result, err := b.client.UploadPartCopy(grpCtx, params)
			completedParts[i] = types.CompletedPart{
				PartNumber: partNum,
				ETag:       result.CopyPartResult.ETag,
			}
			return err
		})
	}
	if err := grp.Wait(); err != nil {
		params := &s3v2.AbortMultipartUploadInput{
			Bucket:   &b.bucket,
			Key:      &dst,
			UploadId: newUp.UploadId,
		}
		_, abortErr := b.client.AbortMultipartUpload(ctx, params)
		return errors.Join(err, abortErr)
	}
	finalParams := &s3v2.CompleteMultipartUploadInput{
		Bucket:   &b.bucket,
		Key:      &dst,
		UploadId: newUp.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	}
	_, err = b.client.CompleteMultipartUpload(ctx, finalParams)
	return err
}

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

type countReader struct {
	io.Reader
	size int64
}

func (r *countReader) Read(p []byte) (int, error) {
	s, err := r.Reader.Read(p)
	r.size += int64(s)
	return s, err
}

func pathErr(op string, path string, err error) error {
	return &fs.PathError{Op: op, Path: path, Err: err}
}
