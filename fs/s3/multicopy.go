package s3

import (
	"context"
	"errors"

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
	// PartSize sets the size of the object parts used for multipart object
	// copy. If it is below the SDK's minimum upload part size, defaultCopyPartSize
	// is used instead. If the resulting part size is too small to copy the
	// object within the max number of parts, the part size is increased in 1
	// MiB increments until it fits.
	PartSize int64
	// Concurrency sets the number of goroutines per copy for copying object
	// parts. Defaults to defaultCopyPartConcurrency (6) when less than 1.
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

// Copy copies the object at src to dst within buck using S3's multipart
// UploadPartCopy API. PartSize and Concurrency are read from c but never
// written back to it: Copy defaults an out-of-range value into a local for
// the duration of the call, so one MultiCopier is safe to share and call
// concurrently.
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
		err = pathErr("copy", src, ErrNoContentLength)
		return
	}
	srcSize = *srcHead.ContentLength
	partSize := c.PartSize
	if partSize < manager.MinUploadPartSize {
		partSize = defaultCopyPartSize
	}
	concurrency := c.Concurrency
	if concurrency < 1 {
		concurrency = defaultCopyPartConcurrency
	}
	psize, partCount := adjustPartSize(srcSize, partSize, manager.MaxUploadParts)
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
	grp.SetLimit(concurrency)
	copySource := encodeCopySource(buck, src)
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
