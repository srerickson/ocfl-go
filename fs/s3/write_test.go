package s3_test

// Tests for write.go: BucketFS.Write and WriteWithOptions through the public
// API, including the shared cross-backend Write contract. Cases that call the
// unexported write() directly live in write_internal_test.go.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

func TestWriteReadDeleteFile(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Log("s3 test service is not running")
		return
	}
	ctx := t.Context()
	fsys := testutil.TmpS3FS(t, nil)
	key := "dir/test-data"
	buff := mock.RandBytes(15 * megabyte)
	n, err := fsys.Write(ctx, key, bytes.NewReader(buff))
	be.NilErr(t, err)
	be.Equal(t, len(buff), int(n))
	for entry, err := range fsys.DirEntries(ctx, "dir") {
		be.NilErr(t, err)
		be.Equal(t, "test-data", entry.Name())
	}
	f, err := fsys.OpenFile(ctx, key)
	be.NilErr(t, err)
	outBytes, err := io.ReadAll(f)
	be.NilErr(t, err)
	be.True(t, bytes.Equal(outBytes, buff))
	be.NilErr(t, fsys.Remove(ctx, key))
}

func TestWriteWithOptions(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Log("s3 test service is not running")
		return
	}
	ctx := t.Context()
	fsys := testutil.TmpS3FS(t, nil)
	// option to require key to not exist
	opt := func(input *s3v2.PutObjectInput) {
		match := "*"
		input.IfNoneMatch = &match
	}
	key := "file"
	body := strings.NewReader("content")
	// first write creates the file
	_, err := fsys.WriteWithOptions(ctx, key, body, opt)
	be.NilErr(t, err)

	// second write fails because key exists
	_, err = fsys.WriteWithOptions(ctx, key, body, opt)
	be.Nonzero(t, err)
	var apiErr smithy.APIError
	be.True(t, errors.As(err, &apiErr))
	be.Equal(t, "PreconditionFailed", apiErr.ErrorCode())
}

func TestWrite_Mock(t *testing.T) {
	ctx := context.Background()
	bodySize := 201 * megabyte
	body := mock.RandBytes(int64(bodySize))
	type testCase struct {
		desc        string
		bucket      string
		key         string
		body        io.Reader
		uploadConc  int
		uploadPSize int64
		mock        func(*testing.T) *mock.S3API
		expect      func(*testing.T, *mock.S3API, int64, error)
	}
	cases := []testCase{
		{
			desc: "invalid path",
			key:  "../file.txt",
			expect: func(t *testing.T, _ *mock.S3API, size int64, err error) {
				isInvalidPathError(t, err)
			},
		}, {
			desc:   "small write",
			bucket: bucket,
			key:    "tmp",
			body:   strings.NewReader("some content"),
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket)
			},
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.NilErr(t, err)
				be.Nonzero(t, state.UpdatedETags["tmp"])

			},
		}, {
			desc:        "multipart",
			bucket:      bucket,
			key:         "tmp",
			uploadPSize: partSize,
			body:        bytes.NewReader(body),
			mock: func(t *testing.T) *mock.S3API {
				api := mock.New(bucket)
				return api
			},
			expect: func(t *testing.T, state *mock.S3API, size int64, err error) {
				be.NilErr(t, err)
				be.Equal(t, int64(bodySize), size)
				expectETag := mock.ETag(body, partSize)
				be.Equal(t, expectETag, state.UpdatedETags["tmp"])
				be.Equal(t, bodySize/partSize+1, state.PartCount())
				be.Equal(t, true, state.MPUComplete)
			},
		},
	}
	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api *mock.S3API
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			uploaderOpt := func(u *manager.Uploader) {
				u.Concurrency = tcase.uploadConc
				u.PartSize = tcase.uploadPSize
			}
			fsys := s3.NewBucketFS(api, tcase.bucket, s3.WithUploaderOptions(uploaderOpt))
			val, err := fsys.Write(ctx, tcase.key, tcase.body)
			tcase.expect(t, api, val, err)
		})
	}
}

// TestWriteFSWriteContract_S3 runs the shared WriteFS.Write contract against
// the S3 backend, using the in-process mock so it runs in CI without a store.
func TestWriteFSWriteContract_S3(t *testing.T) {
	fsys := s3.NewBucketFS(mock.New(bucket), bucket)
	testutil.TestWriteFSWriteContract(t, fsys, testutil.WriteFSWriteContract{
		WriteDotIsError: true,
	})
}

// restoreFailReader is an io.ReadSeeker whose third Seek call — the sniff's
// restore seek back to the original position — always fails. It models a
// reader whose position cannot be restored after the length probe seeks it
// to the end; with the pre-fix code this situation silently uploaded an
// empty body from EOF.
type restoreFailReader struct {
	r *strings.Reader
	n int
}

func (r *restoreFailReader) Read(p []byte) (int, error) { return r.r.Read(p) }

func (r *restoreFailReader) Seek(offset int64, whence int) (int64, error) {
	r.n++
	// sniffSeekerLength seeks in this order: current position, end,
	// restore. Failing the third leaves the reader stranded at EOF.
	if r.n == 3 {
		return 0, errors.New("restore seek failed (injected)")
	}
	return r.r.Seek(offset, whence)
}

// TestWriteContentLengthIntegration_PartiallyConsumedFile exercises the full
// s3 upload path (public BucketFS.Write -> ContentLength sniff -> real HTTP
// PutObject against the test store) with a partially-consumed *os.File,
// which implements both fs.File and io.Seeker. The seeker REMAINING length
// must win over fs.File.Stat's total size so the declared ContentLength
// equals the delivered body; net/http rejects requests where the declared
// length exceeds the bytes actually delivered, so a nil error from Write
// proves the request was accepted. The object must then store exactly the
// remaining bytes.
func TestWriteContentLengthIntegration_PartiallyConsumedFile(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Skip("s3 test service is not running: set $OCFL_TEST_S3 to enable")
	}
	ctx := context.Background()
	fsys := testutil.TmpS3FS(t, nil)

	content := []byte("the quick brown fox jumps over the lazy dog")
	const consume = 10
	path := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// Partially consume the file: the upload must deliver only content[10:].
	if _, err := io.ReadFull(f, make([]byte, consume)); err != nil {
		t.Fatal(err)
	}

	key := "partial-file-upload"
	wantN := int64(len(content) - consume)
	n, err := fsys.Write(ctx, key, f)
	be.NilErr(t, err)
	be.Equal(t, wantN, n)

	// The reader must be left exactly at EOF: the caller consumed 10 bytes
	// and the write consumed the remaining tail. Any other position means
	// the sniff skipped or re-read bytes.
	pos, err := f.Seek(0, io.SeekCurrent)
	be.NilErr(t, err)
	be.Equal(t, int64(len(content)), pos)

	// Read the object back: matching content proves the request carried the
	// remaining bytes, so declared ContentLength and delivered body agreed
	// on the wire and net/http did not reject the upload.
	got, err := fsys.OpenFile(ctx, key)
	be.NilErr(t, err)
	defer got.Close()
	body, err := io.ReadAll(got)
	be.NilErr(t, err)
	be.DeepEqual(t, content[consume:], body)
	info, err := got.Stat()
	be.NilErr(t, err)
	be.Equal(t, wantN, info.Size())
}

// TestWriteContentLengthIntegration_RestoreFailure exercises the error path
// for a seeker whose restore seek fails after the length probe seeks it to
// the end. write() must surface the failure as a *fs.PathError mentioning
// the restore — not silently upload an empty body from EOF — and no object
// may be created in the store.
func TestWriteContentLengthIntegration_RestoreFailure(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Skip("s3 test service is not running: set $OCFL_TEST_S3 to enable")
	}
	ctx := context.Background()
	fsys := testutil.TmpS3FS(t, nil)

	key := "restore-failure-key"
	r := &restoreFailReader{r: strings.NewReader("0123456789")}
	if _, err := fsys.Write(ctx, key, r); err == nil {
		t.Fatal("Write returned nil error when the restore seek failed")
	} else {
		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "write", pathErr.Op)
		be.Equal(t, key, pathErr.Path)
		be.True(t, strings.Contains(pathErr.Err.Error(), "restore"))
	}

	// The failed sniff must abort before any PutObject: nothing may exist
	// at the key (pre-fix behavior created an empty object).
	if _, err := fsys.OpenFile(ctx, key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("object exists after failed write, want fs.ErrNotExist (err = %v)", err)
	}
}
