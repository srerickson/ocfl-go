package s3fs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

const fiveGB = 5_368_709_120

func (b *Backend) Write(p string, buffer io.Reader) (int64, error) {
	if !fs.ValidPath(p) {
		return 0, &fs.PathError{
			Op:   "write",
			Path: p,
			Err:  errors.New("invalid path"),
		}
	}
	// check each path element: parent directories shouldn't be object; the
	// object shouldn't be a directory
	parts := strings.Split(p, "/")
	for i := range parts {
		part := strings.Join(parts[:i+1], "/")
		if i == len(parts)-1 {
			isDir, err := b.isDir(part)
			if err == nil && isDir {
				return 0, &fs.PathError{
					Op:   "write",
					Path: p,
					Err:  fmt.Errorf("exists as directory: %s", part),
				}
			}
		} else {
			isObj, err := b.isObject(part)
			if err == nil && isObj {
				return 0, &fs.PathError{
					Op:   "write",
					Path: p,
					Err:  fmt.Errorf("exists as object: %s", part),
				}
			}
		}
	}
	uploader := s3manager.NewUploaderWithClient(b.cl)
	in := &s3manager.UploadInput{
		Body:   buffer,
		Bucket: &b.bucket,
		Key:    &p,
	}
	_, err := uploader.Upload(in)
	if err != nil {
		return 0, &fs.PathError{Op: "write", Path: p, Err: err}
	}
	headIn := &s3.HeadObjectInput{
		Bucket: &b.bucket,
		Key:    &p,
	}
	resp, err := b.cl.HeadObject(headIn)
	if err != nil {
		return 0, &fs.PathError{Op: "write", Path: p, Err: err}
	}
	return *resp.ContentLength, nil
}

func (b *Backend) Copy(dst, src string) error {
	for _, p := range [2]string{dst, src} {
		if !fs.ValidPath(p) {
			return &fs.PathError{
				Op:   "copy",
				Path: p,
				Err:  errors.New("invalid path"),
			}
		}
	}
	isDir, err := b.isDir(dst)
	if err == nil && isDir {
		return &fs.PathError{
			Op:   "copy",
			Path: dst,
			Err:  errors.New("path is a directory"),
		}
	}
	info, err := fs.Stat(b, src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return &fs.PathError{
			Op:   "copy",
			Path: src,
			Err:  errors.New("not an object"),
		}
	}
	if info.Size() > fiveGB {
		return &fs.PathError{
			Op:   "copy",
			Path: src,
			Err:  errors.New("copies larger than 5GB not supported"),
		}
	}
	in := &s3.CopyObjectInput{
		CopySource: aws.String(path.Join(b.bucket, src)),
		Key:        &dst,
		Bucket:     &b.bucket,
	}
	_, err = b.cl.CopyObject(in)
	if err != nil {
		return &fs.PathError{
			Op:   "copy",
			Path: src,
			Err:  err,
		}
	}
	return nil
}

func (b *Backend) RemoveAll(p string) error {
	if !fs.ValidPath(p) {
		return &fs.PathError{
			Op:   "remove_all",
			Path: p,
			Err:  errors.New("invalid path"),
		}
	}
	if p == "." {
		return &fs.PathError{
			Op:   "remove_all",
			Path: p,
			Err:  errors.New("cannot remove root directory"),
		}
	}
	info, err := fs.Stat(b, p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		in := &s3.DeleteObjectInput{
			Bucket: &b.bucket,
			Key:    &p,
		}
		_, err := b.cl.DeleteObject(in)
		if err != nil {
			return &fs.PathError{
				Op:   "RemoveAll",
				Path: p,
				Err:  err,
			}
		}
	}
	p = p + "/"
	in := &s3.ListObjectsV2Input{
		Prefix: &p,
		Bucket: &b.bucket,
	}
	var pageErr error
	eachPage := func(out *s3.ListObjectsV2Output, last bool) bool {
		for _, o := range out.Contents {
			in := &s3.DeleteObjectInput{
				Bucket: &b.bucket,
				Key:    o.Key,
			}
			_, err := b.cl.DeleteObject(in)
			if err != nil {
				pageErr = err
				return true
			}
		}
		return last
	}
	err = b.cl.ListObjectsV2Pages(in, eachPage)
	if err != nil {
		return err
	}
	return pageErr
}

func (b *Backend) isObject(p string) (bool, error) {
	in := &s3.HeadObjectInput{
		Bucket: &b.bucket,
		Key:    aws.String(p),
	}
	_, err := b.cl.HeadObject(in)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (b *Backend) isDir(p string) (bool, error) {
	in := &s3.ListObjectsV2Input{
		Bucket:    &b.bucket,
		Prefix:    aws.String(p + "/"),
		Delimiter: aws.String("/"),
		MaxKeys:   aws.Int64(1),
	}
	out, err := b.cl.ListObjectsV2(in)
	if err != nil {
		return false, err
	}
	return len(out.CommonPrefixes) > 0 ||
		len(out.Contents) > 0, nil
}
