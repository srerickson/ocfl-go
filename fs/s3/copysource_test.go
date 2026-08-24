package s3_test

// Tests for copysource.go through the public API: keys whose encoding the
// x-amz-copy-source header is sensitive to must survive a real copy. The
// encoding function itself is tested in copysource_internal_test.go.

import (
	"bytes"
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
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

// TestCopySourceSpecialKeys_Integration creates a source object with a key
// containing spaces, unicode, a nested slash (and other special characters),
// copies it to a destination via CopySource (BucketFS.Copy -> CopyObject),
// and fails if the copy fails or the copied content does not match.
func TestCopySourceSpecialKeys_Integration(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Skip("s3 test service is not running: set $OCFL_TEST_S3 to enable")
	}
	ctx := context.Background()
	// creates a random tmp bucket; the bucket and all objects are removed in
	// a t.Cleanup callback.
	fsys := testutil.TmpS3FS(t, nil)

	cases := []struct {
		name string
		src  string
		dst  string
	}{
		{
			name: "spaces",
			src:  "dir with space/file name with spaces.txt",
			dst:  "copy dir/result file with spaces.txt",
		}, {
			name: "unicode",
			src:  "unicode dir/naïve-日本語-文件.txt",
			dst:  "copy/unicode/émoji resumé.txt",
		}, {
			name: "spaces-unicode-nested-slash",
			src:  "dir one/naïve 日本語 file.txt",
			dst:  "copy one/result 日本語.txt",
		}, {
			name: "plus",
			src:  "plus dir/a+b file.txt",
			dst:  "copy/plus/result + file.txt",
		}, {
			name: "percent-hash-question",
			src:  "pct dir/100% done #1?.txt",
			dst:  "copy/pct/# result?.txt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// content includes non-UTF8 bytes so the comparison exercises raw
			// byte equality, not just text round-tripping.
			content := []byte("content for " + tc.src + "\n\x00\x01\x02raw bytes\xff\n")
			n, err := fsys.Write(ctx, tc.src, bytes.NewReader(content))
			be.NilErr(t, err)
			be.Equal(t, int64(len(content)), n)

			// the CopySource site under test
			size, err := fsys.Copy(ctx, tc.dst, tc.src)
			be.NilErr(t, err) // copy failed: CopySource mis-encoded (e.g. ' ' -> '+')
			be.Equal(t, int64(len(content)), size)

			got, err := ocflfs.ReadAll(ctx, fsys, tc.dst)
			be.NilErr(t, err)
			be.Equal(t, string(content), string(got))
		})
	}
}

// TestCopySourceSpecialKeys_Multipart_Integration exercises the second
// CopySource construction site, UploadPartCopy in multicopy.go, with the same
// class of special keys. BucketFS.Copy only falls back to the multipart path
// when CopyObject reports the source is too large for a single copy (>5 GiB),
// which is impractical in an integration test, so the multipart copier is
// driven directly. Both sites build CopySource with the same encoding
// expression, so this verifies that expression on the UploadPartCopy wire
// path.
func TestCopySourceSpecialKeys_Multipart_Integration(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Skip("s3 test service is not running: set $OCFL_TEST_S3 to enable")
	}
	ctx := context.Background()
	fsys := testutil.TmpS3FS(t, nil)

	// 6 MiB parts with a source 100 bytes larger than one part forces a
	// two-part UploadPartCopy; the first part is a full ≥5 MiB part and the
	// second is a legal small final part. A source >5 MiB is also required
	// for CopySourceRange requests.
	const partSize = 6 * 1024 * 1024
	srcSize := partSize + 100
	content := make([]byte, srcSize)
	for i := range content {
		content[i] = byte(i % 251)
	}
	src := "mp dir/naïve 日本語 src.bin"
	dst := "mp copy/result ファイル.bin"

	n, err := fsys.Write(ctx, src, bytes.NewReader(content))
	be.NilErr(t, err)
	be.Equal(t, int64(srcSize), n)

	copier := s3.NewMultiCopier(fsys.Client(), func(mc *s3.MultiCopier) {
		mc.PartSize = partSize
	})
	size, err := copier.Copy(ctx, fsys.Bucket(), dst, src)
	be.NilErr(t, err) // multipart copy failed: UploadPartCopy CopySource mis-encoded
	be.Equal(t, int64(srcSize), size)

	got, err := ocflfs.ReadAll(ctx, fsys, dst)
	be.NilErr(t, err)
	be.Equal(t, string(content), string(got))
}
