package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"time"

	// awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	// awsv2cfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	s3mgr "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	// "github.com/aws/smithy-go"
)

func New() {}

type S3Backend struct {
	bucket string
	client *s3v2.Client
}

func (b *S3Backend) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
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
	return &s3File{key: name, obj: obj}, nil
}

func (b *S3Backend) ReadDir(ctx context.Context, dir string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(dir) {
		return nil, &fs.PathError{Op: "readdir", Path: dir, Err: fs.ErrInvalid}
	}
	params := &s3v2.ListObjectsV2Input{
		Bucket:    &b.bucket,
		Delimiter: aws.String("/"),
		MaxKeys:   aws.Int32(1000),
	}
	if dir != "." {
		params.Prefix = aws.String(dir + "/")
	}
	entries := make([]fs.DirEntry, 0, 32)
	for {
		list, err := b.client.ListObjectsV2(ctx, params)
		if err != nil {
			return nil, &fs.PathError{
				Op:   "readdir",
				Path: dir,
				Err:  err,
			}
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
		if !*list.IsTruncated {
			break
		}
		params.ContinuationToken = list.NextContinuationToken
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		aN, bN := a.Name(), b.Name()
		switch {
		case aN < bN:
			return -1
		case aN > bN:
			return 1
		default:
			return 0
		}
	})
	return entries, nil
}

func (b *S3Backend) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	uploader := s3mgr.NewUploader(b.client)
	countReader := &countReader{Reader: r}
	params := &s3v2.PutObjectInput{
		Bucket: &b.bucket,
		Key:    &name,
		Body:   countReader,
	}
	_, err := uploader.Upload(ctx, params)
	if err != nil {
		return 0, err
	}
	return countReader.size, nil
}

type s3File struct {
	key string
	obj *s3v2.GetObjectOutput
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
