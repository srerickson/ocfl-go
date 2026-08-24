package s3_test

import (
	"context"
	"errors"
	"testing"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/carlmjohnson/be"

	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

// maxCopyObjectSize mirrors the unexported maxCopySize constant in
// fs/s3/s3.go. copy() routes a source to MultiCopier only when the HEAD
// ContentLength strictly exceeds this value. Keep in sync with the
// implementation.
const maxCopyObjectSize = int64(5 * 1024 * 1024 * 1024) // 5 GiB

// newCopyFS returns a BucketFS over api whose MultiCopier uses partSize
// (0 keeps the library default). copyPSize is capped so multipart copies of
// the large virtual objects stay cheap.
func newCopyFS(api *mock.S3API, partSize int64) *s3.BucketFS {
	return s3.NewBucketFS(api, bucket, s3.WithMultiPartCopyOption(func(mc *s3.MultiCopier) {
		mc.Concurrency = 2
		if partSize > 0 {
			mc.PartSize = partSize
		}
	}))
}

// TestCopy_LargeSourceUsesMultiPart is a regression test for the copy()
// strategy change: the copy path must be chosen from the HEAD ContentLength
// (> maxCopySize) BEFORE CopyObject is attempted, not from CopyObject
// failing with the AWS-specific "copy source is larger than the maximum
// allowable size" error string. S3-compatible stores (MinIO, GCS, ...) are
// not required to produce that message, so a large copy must succeed there
// without ever calling CopyObject.
func TestCopy_LargeSourceUsesMultiPart(t *testing.T) {
	ctx := context.Background()
	const srcSize = maxCopyObjectSize + 1
	api := mock.New(bucket, &mock.Object{
		Key:           "big-src",
		ContentLength: srcSize, // virtual object: no 5 GiB body in memory
	})
	fsys := newCopyFS(api, 32*megabyte)
	size, err := fsys.Copy(ctx, "big-dst", "big-src")
	be.NilErr(t, err)
	be.Equal(t, srcSize, size) // reports the full source size
	be.Equal(t, 0, api.CopyObjectCalls)
	be.True(t, api.MPUCreated)
	be.True(t, api.MPUComplete)
	be.True(t, api.PartCount() > 0)
	be.Nonzero(t, api.UpdatedETags["big-dst"])
}

// TestCopy_SmallSourceUsesCopyObject checks that an ordinary (<= 5 GiB)
// source goes through a single CopyObject request with no multipart
// machinery involved.
func TestCopy_SmallSourceUsesCopyObject(t *testing.T) {
	ctx := context.Background()
	body := []byte("small object content")
	api := mock.New(bucket, &mock.Object{Key: "small-src", Body: body})
	fsys := newCopyFS(api, 0)
	size, err := fsys.Copy(ctx, "small-dst", "small-src")
	be.NilErr(t, err)
	be.Equal(t, int64(len(body)), size)
	be.Equal(t, 1, api.CopyObjectCalls)
	be.False(t, api.MPUCreated)
	be.Equal(t, 0, api.PartCount())
	be.Nonzero(t, api.UpdatedETags["small-dst"])
}

// TestCopy_ExactlyMaxCopySizeUsesCopyObject pins the boundary condition:
// the multipart branch is ContentLength > maxCopySize (strictly greater),
// so an object exactly at the 5 GiB limit must still use CopyObject.
func TestCopy_ExactlyMaxCopySizeUsesCopyObject(t *testing.T) {
	ctx := context.Background()
	api := mock.New(bucket, &mock.Object{
		Key:           "limit-src",
		ContentLength: maxCopyObjectSize,
	})
	fsys := newCopyFS(api, 0)
	size, err := fsys.Copy(ctx, "limit-dst", "limit-src")
	be.NilErr(t, err)
	be.Equal(t, maxCopyObjectSize, size)
	be.Equal(t, 1, api.CopyObjectCalls)
	be.False(t, api.MPUCreated)
}

// TestCopy_CopyObjectErrorPropagates pins that a CopyObject failure on a
// small source is returned unchanged, with no retry as a multipart copy. The
// strategy choice is made from the source's declared size alone; matching on
// the error text instead would be wrong for S3-compatible stores that phrase
// the same condition differently. The mock returns the exact AWS "copy source
// is larger than the maximum allowable size" text, so a reintroduced string
// match would be caught here rather than in production against MinIO.
func TestCopy_CopyObjectErrorPropagates(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("copy source is larger than the maximum allowable size")
	api := mock.New(bucket, &mock.Object{Key: "small-src", Body: []byte("data")})
	api.CopyObjectFunc = func(_ context.Context, _ *s3v2.CopyObjectInput, _ ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error) {
		return nil, wantErr
	}
	fsys := newCopyFS(api, 0)
	_, err := fsys.Copy(ctx, "small-dst", "small-src")
	be.Nonzero(t, err)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the CopyObject error to propagate unchanged, got: %v", err)
	}
	be.Equal(t, 1, api.CopyObjectCalls)
	be.False(t, api.MPUCreated)
}
