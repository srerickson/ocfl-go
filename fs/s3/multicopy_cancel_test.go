package s3_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/carlmjohnson/be"

	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

// cancelAwareAPI wraps mock.S3API so that cancellation behaves like a real
// S3 client: every multipart method fails immediately with ctx.Err() when
// the passed context is already canceled. The mock itself ignores contexts,
// so without this wrapper the bug this test guards against (deferred
// Abort/CompleteMultipartUpload running on the caller's canceled ctx) would
// appear to succeed.
//
// partCopyHook, when set, runs after every successful UploadPartCopy; tests
// use it to cancel the caller's context at a precise, deterministic point of
// the part loop (Concurrency is pinned to 1 so parts run serially).
type cancelAwareAPI struct {
	*mock.S3API
	cancel       context.CancelFunc
	partCopyHook func()
	partCopies   atomic.Int32
}

func (a *cancelAwareAPI) AbortMultipartUpload(ctx context.Context, in *s3v2.AbortMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.AbortMultipartUploadOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return a.S3API.AbortMultipartUpload(ctx, in, opts...)
}

func (a *cancelAwareAPI) CompleteMultipartUpload(ctx context.Context, in *s3v2.CompleteMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CompleteMultipartUploadOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return a.S3API.CompleteMultipartUpload(ctx, in, opts...)
}

func (a *cancelAwareAPI) UploadPartCopy(ctx context.Context, in *s3v2.UploadPartCopyInput, opts ...func(*s3v2.Options)) (*s3v2.UploadPartCopyOutput, error) {
	// A canceled caller ctx must stop the part loop too: this is what
	// drives Copy into the abort branch under a cancellation.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out, err := a.S3API.UploadPartCopy(ctx, in, opts...)
	if err == nil {
		a.partCopies.Add(1)
		if a.partCopyHook != nil {
			a.partCopyHook()
		}
	}
	return out, err
}

// serialCopier returns a MultiCopier over api pinned to one worker so part
// copies (and the partCopyHook) run in a deterministic order.
func serialCopier(api s3.MultiCopyAPI) *s3.MultiCopier {
	return s3.NewMultiCopier(api, func(mc *s3.MultiCopier) {
		mc.PartSize = partSize
		mc.Concurrency = 1
	})
}

// TestMultiCopy_AbortSurvivesCancellation pins the cleanup path in
// multicopy.go: when the caller cancels the context mid-copy the part loop
// fails, and the deferred AbortMultipartUpload must still reach S3. It
// therefore runs on a context derived with context.WithoutCancel — issuing
// the abort on the caller's canceled ctx would fail immediately and orphan
// the multipart upload along with every part already uploaded.
func TestMultiCopy_AbortSurvivesCancellation(t *testing.T) {
	const src = "big-src"
	body := mock.RandBytes(13 * megabyte) // 3 parts at partSize
	api := mock.New(bucket, &mock.Object{Key: src, Body: body})
	ctx, cancel := context.WithCancel(context.Background())
	wrapped := &cancelAwareAPI{S3API: api, cancel: cancel}
	// Cancel the caller's ctx right after the first part copy completes;
	// with Concurrency=1 every subsequent part fails on the canceled
	// grpCtx, driving Copy into the abort branch.
	wrapped.partCopyHook = func() { cancel() }

	_, err := serialCopier(wrapped).Copy(ctx, bucket, "dst-file", src)
	// the copy itself must report the caller's cancellation...
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the copy to fail with context.Canceled, got: %v", err)
	}
	// ...but the MPU must still have been aborted, not orphaned
	be.True(t, api.MPUAborted)
	be.False(t, api.MPUComplete)
}

// TestMultiCopy_CompleteSurvivesCancellation is the success-path
// counterpart: the caller cancels only after every part has been copied, so
// Copy is about to complete the MPU. The deferred CompleteMultipartUpload
// must still run on a non-canceled context, or the fully-uploaded MPU is
// stranded forever. The copy must succeed and produce a real destination
// object.
func TestMultiCopy_CompleteSurvivesCancellation(t *testing.T) {
	const src = "big-src"
	body := mock.RandBytes(13 * megabyte) // 3 parts at partSize
	api := mock.New(bucket, &mock.Object{Key: src, Body: body})
	ctx, cancel := context.WithCancel(context.Background())
	wrapped := &cancelAwareAPI{S3API: api, cancel: cancel}
	// Cancel only after the final part copy: the part loop finishes
	// cleanly and only the deferred CompleteMultipartUpload runs under a
	// canceled caller ctx.
	wrapped.partCopyHook = func() {
		if wrapped.partCopies.Load() == 3 {
			cancel()
		}
	}

	size, err := serialCopier(wrapped).Copy(ctx, bucket, "dst-file", src)
	be.NilErr(t, err) // complete must succeed despite the canceled caller ctx
	be.Equal(t, int64(len(body)), size)
	be.True(t, api.MPUComplete)
	be.False(t, api.MPUAborted)
	be.Equal(t, mock.ETag(body, partSize), api.UpdatedETags["dst-file"])
}
