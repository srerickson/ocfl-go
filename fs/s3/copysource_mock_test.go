package s3_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

// trickyKeys are OCFL-legal object keys (io/fs.ValidPath accepts all of
// them) that a query-string encoder such as url.QueryEscape would mangle,
// turning a space into '+' and every '/' into %2F. Expected values are the
// exact x-amz-copy-source strings S3 must receive: "bucket/key" with '/'
// literal and each segment percent-encoded exactly once (%20 for space, %2B
// for '+', %25 for '%', UTF-8 %XX for non-ASCII).
var trickyKeys = []struct {
	key  string
	want string
}{
	{key: "my file.txt", want: "ocfl-go-test/my%20file.txt"},
	{key: "dir/sub dir/file with space.txt", want: "ocfl-go-test/dir/sub%20dir/file%20with%20space.txt"},
	{key: "file+plus.txt", want: "ocfl-go-test/file%2Bplus.txt"},
	{key: "100%.txt", want: "ocfl-go-test/100%25.txt"},
	{key: "file%20name.txt", want: "ocfl-go-test/file%2520name.txt"},
	{key: "why#what?x", want: "ocfl-go-test/why%23what%3Fx"},
	{key: "naïve-日本語/emoji-😀.txt", want: "ocfl-go-test/na%C3%AFve-%E6%97%A5%E6%9C%AC%E8%AA%9E/emoji-%F0%9F%98%80.txt"},
}

// TestCopySource_WireValue_Mock captures the exact CopySource value the S3
// API receives for a single-part copy and asserts it is the per-segment
// encoded form (no '+' for spaces, literal '/' separators, single encoding).
func TestCopySource_WireValue_Mock(t *testing.T) {
	ctx := context.Background()
	for _, tc := range trickyKeys {
		t.Run(tc.key, func(t *testing.T) {
			api := mock.New(bucket, &mock.Object{Key: tc.key, Body: []byte("some content")})
			var got *string
			api.CopyObjectFunc = func(_ context.Context, in *s3v2.CopyObjectInput, _ ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error) {
				got = in.CopySource
				return &s3v2.CopyObjectOutput{}, nil
			}
			fsys := s3.NewBucketFS(api, bucket)
			_, err := fsys.Copy(ctx, "dst-file", tc.key)
			be.NilErr(t, err)
			be.Nonzero(t, got)
			be.Equal(t, tc.want, *got)
		})
	}
}

// TestCopySource_RoundTrip_Mock copies objects whose keys contain spaces,
// unicode, '+', '%' and nested slashes through the mock's default single-
// part CopyObject. The mock decodes the copy source with url.PathUnescape
// (mirroring S3, which does not treat '+' as space) and looks the decoded
// key up verbatim, so a successful copy with the right payload proves the
// wire value round-trips to the exact raw key — no double-encoding, no
// corruption.
func TestCopySource_RoundTrip_Mock(t *testing.T) {
	ctx := context.Background()
	for _, tc := range trickyKeys {
		t.Run(tc.key, func(t *testing.T) {
			body := []byte("some content with a space and unicode: naïve-日本語")
			api := mock.New(bucket, &mock.Object{Key: tc.key, Body: body})
			fsys := s3.NewBucketFS(api, bucket)
			size, err := fsys.Copy(ctx, "dst-file", tc.key)
			be.NilErr(t, err)
			be.Equal(t, int64(len(body)), size)
			be.Equal(t, 0, api.PartCount())
			// exact payload match for the destination etag
			be.Equal(t, `"`+mock.ETag(body, partSize)+`"`, api.UpdatedETags["dst-file"])
		})
	}
}

// TestCopySource_RoundTrip_Multipart_Mock exercises the multipart copy path
// (UploadPartCopy, multicopy.go) with a key containing spaces and nested
// slashes. The per-part copy source must resolve to the same object: if the
// encoding were wrong ('+' for space or %2F for slash), the mock's decode
// would produce a key that does not exist and the copy would fail.
//
// The multipart copier is driven directly (rather than via BucketFS.Copy,
// whose multipart trigger is ContentLength-based) so the test does not need
// a >5GiB payload.
func TestCopySource_RoundTrip_Multipart_Mock(t *testing.T) {
	ctx := context.Background()
	const src = "dir/sub dir/file with space.txt"
	body := mock.RandBytes(13 * megabyte) // 3 parts at partSize
	api := mock.New(bucket, &mock.Object{Key: src, Body: body})
	head, err := api.HeadObject(ctx, &s3v2.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(src),
	})
	be.NilErr(t, err)
	copier := s3.NewMultiCopier(api, func(mc *s3.MultiCopier) { mc.PartSize = partSize })
	size, err := copier.Copy(ctx, bucket, "dst-file", src, head)
	be.NilErr(t, err)
	be.Equal(t, int64(len(body)), size)
	be.Equal(t, 3, api.PartCount())
	be.Equal(t, mock.ETag(body, partSize), api.UpdatedETags["dst-file"])
}
