package s3_test

// Integration tests for S3 CopySource with "special" object keys: spaces,
// unicode, nested slashes, and other characters that are legal OCFL content
// paths (io/fs.ValidPath allows them) but require URL-encoding in the
// x-amz-copy-source header.
//
// Run against an S3-compatible store (e.g. MinIO) by setting $OCFL_TEST_S3 to
// its endpoint, e.g.:
//
//	OCFL_TEST_S3=http://127.0.0.1:9000 AWS_ACCESS_KEY_ID=... \
//	AWS_SECRET_ACCESS_KEY=... AWS_REGION=us-east-1 \
//	go test ./fs/s3 -run TestCopySourceSpecialKeys -v
//
// The same command runs against real S3 when $OCFL_TEST_S3 points at a real
// S3 endpoint (e.g. https://s3.us-east-1.amazonaws.com; path-style requests
// are still supported for us-east-1) and AWS credentials for that account are
// in the environment. Buckets are created per test and removed by
// t.Cleanup via testutil.RemoveBucket.
//
// Why an integration test and not just a unit test: the encoding is only
// wrong on the wire, and only some stores reject it. Both CopySource
// construction sites (CopyObject in s3.go and UploadPartCopy in multicopy.go)
// go through copySourcePath in copysource.go, which percent-encodes each
// segment minio-go style: literal '/' separators, space -> %20, '+' -> %2B.
//
// Two encodings that look reasonable are not. A whole-string url.QueryEscape
// encodes spaces as '+' and every '/' as %2F; verified against MinIO, that
// returns 404 for keys with spaces, unicode, or nested slashes. A per-segment
// url.PathEscape fixes those but leaves a literal '+' in the key, which the
// store reads back as a space — so the '+' case below is load-bearing, and it
// is the one that fails silently rather than loudly.

import (
	"bytes"
	"context"
	"testing"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

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
