package s3

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"time"

	// awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	// awsv2cfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	// "github.com/aws/smithy-go"
)

func New() {}

type S3Backend struct {
	bucket string
	client *s3v2.Client
}

func (b *S3Backend) OpenFile(ctx context.Context, pth string) (fs.File, error) {
	if !fs.ValidPath(pth) {
		return nil, &fs.PathError{Op: "open", Path: pth, Err: fs.ErrInvalid}
	}
	params := &s3v2.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(pth),
	}
	obj, err := b.client.GetObject(ctx, params)
	if err != nil {
		fsErr := &fs.PathError{Op: "open", Path: pth}
		var awsErr *types.NoSuchKey
		switch {
		case errors.As(err, &awsErr):
			fsErr.Err = fs.ErrNotExist
		default:
			fsErr.Err = err
		}
		return nil, fsErr
	}
	return &s3File{key: pth, obj: obj}, nil
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
		isDir:   false,
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
	isDir   bool
	sys     any
}

func (i iofsInfo) Name() string       { return i.name }
func (i iofsInfo) Size() int64        { return i.size }
func (i iofsInfo) Mode() fs.FileMode  { return i.mode }
func (i iofsInfo) ModTime() time.Time { return i.modTime }
func (i iofsInfo) IsDir() bool        { return i.isDir }
func (i iofsInfo) Sys() any           { return i.sys }
