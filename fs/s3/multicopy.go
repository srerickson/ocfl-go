package s3

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"golang.org/x/sync/errgroup"
)

const (
	defaultCopyPartConcurrency = 6
	defaultCopyPartSize        = 32 * megabyte
	// mpCleanupTimeout bounds how long the deferred multipart cleanup
	// (abort or complete) may take. Only the cleanup runs under this
	// deadline: the copy itself is bounded by the caller's ctx. It is
	// ample for either single-request cleanup and prevents the defer from
	// hanging indefinitely if the S3 endpoint stops responding.
	mpCleanupTimeout = 30 * time.Second
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
	// Read the receiver's knobs once into locals and apply defaults to the
	// locals: never write to the shared receiver, since a MultiCopier may be
	// reused across concurrent Copy calls.
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
	// The deferred abort/complete must survive caller cancellation: if the
	// caller's ctx is canceled while the parts are being copied, grp.Wait
	// below fails and this defer runs with a canceled ctx, which would fail
	// the cleanup and leave an orphaned MPU with uploaded parts (abort path)
	// or a stranded fully-uploaded MPU (complete path). The cleanup runs on
	// a context derived with context.WithoutCancel — same values, no
	// cancellation and no deadline from the caller — bounded only by
	// mpCleanupTimeout, which starts when the cleanup runs, so a long copy
	// does not eat into it.
	defer func() {
		// complete or abort the multipart upload
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), mpCleanupTimeout)
		defer cleanupCancel()
		switch {
		case err != nil:
			params := &s3.AbortMultipartUploadInput{
				Bucket:   &buck,
				Key:      &dst,
				UploadId: newUp.UploadId,
			}
			_, abortErr := c.api.AbortMultipartUpload(cleanupCtx, params)
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
			_, err = c.api.CompleteMultipartUpload(cleanupCtx, params)
		}
	}()
	grp, grpCtx := errgroup.WithContext(ctx)
	grp.SetLimit(concurrency)
	copySource := copySourcePath(buck, src)
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
