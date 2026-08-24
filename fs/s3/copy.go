package s3

import (
	"context"
	"errors"
	"io/fs"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// maxCopySize is the maximum size of a source object that can be copied
// with a single CopyObject request. Larger objects must be copied in
// parts using MultiCopier.
const maxCopySize int64 = 5 * 1024 * 1024 * 1024

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
