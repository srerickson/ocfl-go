package s3

import (
	"context"
	"errors"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"golang.org/x/sync/errgroup"
)

const (
	defaultCopyPartConcurrency = 6
	defaultCopyPartSize        = 32 * megabyte
)

type MultiCopier struct {
	// PartSize sets the size of the object parts used
	// for multipart object copy. If the part size is too
	// small to be copied using the max number of parts,
	// the part size will be increased in 1 MiB increments
	// until it fits.
	PartSize int64
	// Concurrency stes the number of goroutines
	// per copy for copying object parts. defaults to 12.
	Concurrency int

	api MultiCopyAPI
}

func NewMultiCopier(api MultiCopyAPI, opts ...func(*MultiCopier)) *MultiCopier {
	copier := MultiCopier{
		api: api,
	}
	for _, o := range opts {
		if o != nil {
			o(&copier)
		}
	}
	return &copier
}

func (c *MultiCopier) Copy(ctx context.Context, buck string, dst, src string, srcHeads ...*s3.HeadObjectOutput) (srcSize int64, err error) {
	var srcHead *s3.HeadObjectOutput
	if len(srcHeads) > 0 {
		srcHead = srcHeads[0]
	}
	if srcHead == nil {
		headParams := &s3.HeadObjectInput{Bucket: &buck, Key: &src}
		srcHead, err = c.api.HeadObject(ctx, headParams)
		if err != nil {
			err = pathErr("copy", src, err)
			return
		}
	}
	if srcHead.ContentLength == nil {
		err = pathErr("copy", src, errors.New("missing content length"))
		return
	}
	srcSize = *srcHead.ContentLength
	if c.PartSize < manager.MinUploadPartSize {
		c.PartSize = defaultCopyPartSize
	}
	if c.Concurrency < 1 {
		c.Concurrency = defaultCopyPartConcurrency
	}
	psize, partCount := adjustPartSize(srcSize, c.PartSize, manager.MaxUploadParts)
	completedParts := make([]types.CompletedPart, partCount)
	uploadParams := &s3.CreateMultipartUploadInput{Bucket: &buck, Key: &dst}
	newUp, err := c.api.CreateMultipartUpload(ctx, uploadParams)
	if err != nil {
		err = pathErr("copy", dst, err)
		return
	}
	defer func() {
		// complete or abort the multipart upload
		switch {
		case err != nil:
			params := &s3.AbortMultipartUploadInput{
				Bucket:   &buck,
				Key:      &dst,
				UploadId: newUp.UploadId,
			}
			_, abortErr := c.api.AbortMultipartUpload(ctx, params)
			err = errors.Join(err, abortErr)
		default:
			upload := &types.CompletedMultipartUpload{
				Parts: completedParts,
			}
			params := &s3.CompleteMultipartUploadInput{
				Bucket:          &buck,
				Key:             &dst,
				UploadId:        newUp.UploadId,
				MultipartUpload: upload,
			}
			_, err = c.api.CompleteMultipartUpload(ctx, params)
		}
	}()
	grp, grpCtx := errgroup.WithContext(ctx)
	grp.SetLimit(c.Concurrency)
	copySource := url.QueryEscape(buck + "/" + src)
	for i := range partCount {
		grp.Go(func() error {
			var err error
			partNum := i + 1
			srcRange := byteRange(partNum, psize, srcSize)
			params := &s3.UploadPartCopyInput{
				Bucket:          &buck,
				CopySource:      &copySource,
				Key:             &dst,
				UploadId:        newUp.UploadId,
				PartNumber:      &partNum,
				CopySourceRange: &srcRange,
			}
			result, err := c.api.UploadPartCopy(grpCtx, params)
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
