package s3_test

// Tests for copy.go: BucketFS.Copy, in particular its choice between a
// single CopyObject and a multipart copy. The multipart copier itself is
// tested in multicopy_test.go.

import (
	"context"
	"errors"
	"strconv"
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

func TestCopy_Mock(t *testing.T) {
	ctx := context.Background()
	type testCase struct {
		desc      string
		mock      func(t *testing.T) *mock.S3API
		bucket    string
		copyConc  int
		copyPSize int64
		src       string
		dst       string
		expect    func(*testing.T, *mock.S3API, int64, error)
	}
	cases := []testCase{
		{
			desc: "simple copy",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, &mock.Object{
					Key:  "src-file",
					Body: []byte("some content"),
				})
			},
			bucket: bucket,
			src:    "src-file",
			dst:    "dst-file",
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.NilErr(t, err)
				be.Nonzero(t, state.UpdatedETags["dst-file"])
				be.Nonzero(t, size)
				be.Equal(t, 0, state.PartCount())
			},
		}, {
			desc: "multipart copy",
			mock: func(t *testing.T) *mock.S3API {
				// Virtual source object: HEAD reports ContentLength > the
				// 5 GiB maxCopySize threshold without materializing a body.
				// copy() must route straight to MultiCopier on the declared
				// size alone and never invoke CopyObject: the threshold is a
				// size check, not a fallback driven by a failed CopyObject.
				return mock.New(bucket, &mock.Object{
					Key:           "src-file",
					ContentLength: maxCopyObjectSize + 1,
				})
			},
			bucket:    bucket,
			src:       "src-file",
			dst:       "dst-file",
			copyPSize: partSize,
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.NilErr(t, err)
				be.Equal(t, maxCopyObjectSize+1, size)
				be.Equal(t, 0, state.CopyObjectCalls)
				be.True(t, state.MPUCreated)
				be.True(t, state.MPUComplete)
				be.True(t, state.PartCount() > 0)
				be.Nonzero(t, state.UpdatedETags["dst-file"])
			},
		},
	}
	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api *mock.S3API
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			copyOpts := func(mc *s3.MultiCopier) {
				mc.Concurrency = tcase.copyConc
				mc.PartSize = tcase.copyPSize
			}
			fsys := s3.NewBucketFS(api, tcase.bucket,
				s3.WithMultiPartCopyOption(copyOpts))
			size, err := fsys.Copy(ctx, tcase.dst, tcase.src)
			tcase.expect(t, api, size, err)
		})
	}
}

// TestCopyErrNotExist_Smithy404 verifies the same mapping on the fs.Copy path,
// which also calls HeadObject and errIsNotExist() for its source check.
func TestCopyErrNotExist_Smithy404(t *testing.T) {
	ctx := context.Background()
	orig := smithy404Err()
	api := &headErrAPI{S3API: mock.New(bucket), headErr: orig}
	fsys := s3.NewBucketFS(api, bucket)
	_, err := fsys.Copy(ctx, "dst-file.txt", "missing-src.txt")
	notExistWraps(t, "copy", err, orig)
}

// TestCopy_NilContentLengthError pins the nil guard on the copy strategy
// decision. copy() picks single CopyObject vs multipart from
// srcHead.ContentLength, which a HEAD response is not obliged to set;
// dereferencing it unguarded panics. Mirroring the guard in MultiCopier.Copy,
// copy() must return a "missing content length" error before any CopyObject
// or multipart machinery runs.
func TestCopy_NilContentLengthError(t *testing.T) {
	ctx := context.Background()
	api := &nilLengthAPI{S3API: mock.New(bucket)}
	fsys := s3.NewBucketFS(api, bucket)

	_, err := fsys.Copy(ctx, "dst-key.txt", "src-key.txt")
	missingContentLengthErr(t, "copy", "src-key.txt", err)

	// The guard must fire before the strategy decision: neither the
	// single-CopyObject path nor the multipart path may be attempted.
	be.Equal(t, 0, api.S3API.CopyObjectCalls)
	be.False(t, api.S3API.MPUCreated)
	be.Equal(t, 0, api.S3API.PartCount())
}

// TestNilContentLength_PresentLengthControl pins the stub design: the same
// API shape with a non-nil ContentLength (as real S3 always returns) copies
// and opens normally, so the nil-length stub alone is what changes the
// behavior — not the stub itself.
func TestNilContentLength_PresentLengthControl(t *testing.T) {
	ctx := context.Background()
	body := []byte("hello world")
	api := mock.New(bucket, &mock.Object{Key: "src-key.txt", Body: body})
	fsys := s3.NewBucketFS(api, bucket)

	size, err := fsys.Copy(ctx, "dst-key.txt", "src-key.txt")
	be.NilErr(t, err)
	be.Equal(t, int64(len(body)), size)

	f, err := fsys.OpenFile(ctx, "src-key.txt")
	be.NilErr(t, err)
	defer f.Close()
	fi, err := f.Stat()
	be.NilErr(t, err)
	be.Equal(t, int64(len(body)), fi.Size())
}
