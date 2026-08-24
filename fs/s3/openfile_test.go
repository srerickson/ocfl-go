package s3_test

// Tests for openfile.go: BucketFS.OpenFile and the s3File it returns
// (Read, Seek, Stat, Close). Cases that reach s3File's unexported internals
// live in openfile_internal_test.go.

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"testing"
	"time"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

func TestOpenFile(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Log("s3 test service is not running")
		return
	}
	fixtureFS := ocflfs.DirFS(fixtures)
	fsys := testutil.TmpS3FS(t, fixtureFS)
	type test struct {
		ctx    context.Context
		name   string
		expect func(*testing.T, fs.File, error)
	}
	tests := map[string]test{
		"open file": {
			name: "hello.csv",
			expect: func(t *testing.T, f fs.File, err error) {
				be.NilErr(t, err)
				bytes, err := io.ReadAll(f)
				be.NilErr(t, err)
				be.Equal(t, `1,2,3,"strings"`, string(bytes))
				info, err := f.Stat()
				be.NilErr(t, err)
				fixtureInfo, err := ocflfs.StatFile(context.Background(), fixtureFS, "hello.csv")
				be.NilErr(t, err)
				compareFileInf(t, info, fixtureInfo)
				sys := info.Sys()
				be.Nonzero(t, sys)
				objMeta, isHeadObjectOutput := sys.(*s3v2.HeadObjectOutput)
				be.True(t, isHeadObjectOutput)
				be.Equal(t, *objMeta.ContentLength, info.Size())
				be.Equal(t, *objMeta.LastModified, info.ModTime())
			},
		},
		"open prefix": {
			name: "folder1",
			expect: func(t *testing.T, f fs.File, err error) {
				be.Nonzero(t, err)
				be.True(t, errors.Is(err, fs.ErrNotExist))
			},
		},
		"open missing": {
			name: "missing-file.txt",
			expect: func(t *testing.T, f fs.File, err error) {
				be.Nonzero(t, err)
				be.True(t, errors.Is(err, fs.ErrNotExist))
			},
		},
	}
	for desc, test := range tests {
		t.Run(desc, func(t *testing.T) {
			ctx := test.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			f, err := fsys.OpenFile(ctx, test.name)
			test.expect(t, f, err)
		})
	}

	t.Run("err if modified", func(t *testing.T) {
		// if a file is modified after opening it, read should fail
		ctx := t.Context()
		key := "file"
		body1 := strings.NewReader("content1")
		body2 := strings.NewReader("content2")
		_, err := fsys.Write(ctx, key, body1) // create
		be.NilErr(t, err)
		f, err := fsys.OpenFile(ctx, key) // open
		be.NilErr(t, err)
		_, err = fsys.Write(ctx, key, body2) // modify
		be.NilErr(t, err)
		_, err = io.ReadAll(f) // read fails with "PreconditionFailed"
		be.Nonzero(t, err)
		var apiErr smithy.APIError
		be.True(t, errors.As(err, &apiErr))
		be.Equal(t, "PreconditionFailed", apiErr.ErrorCode())
	})
}

func TestOpenFile_Mock(t *testing.T) {
	type testCase struct {
		desc   string
		bucket string
		key    string
		mock   func(*testing.T) *mock.S3API
		expect func(*testing.T, fs.File, error)
	}
	ctx := context.Background()
	obj := &mock.Object{
		Key:          "dir/file.tiff",
		Body:         []byte("content"),
		LastModified: time.Now(),
	}
	cases := []testCase{
		{
			desc:   "valid input",
			key:    obj.Key,
			bucket: bucket,
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, obj)
			},
			expect: func(t *testing.T, f fs.File, err error) {
				be.NilErr(t, err)
				body, err := io.ReadAll(f)
				be.NilErr(t, err)
				be.DeepEqual(t, obj.Body, body)
				info, err := f.Stat()
				be.NilErr(t, err)
				be.Equal(t, int64(len(body)), info.Size())
				be.Equal(t, obj.LastModified, info.ModTime())
				be.Equal(t, fs.ModeIrregular|0644, info.Mode())
				be.Equal(t, false, info.IsDir())
				be.Nonzero(t, info.Sys())
			},
		}, {
			desc:   "ErrNotExist",
			key:    "missing",
			bucket: bucket,
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket)
			},
			expect: func(t *testing.T, _ fs.File, err error) {
				isPathError(t, err)
				be.True(t, errors.Is(err, fs.ErrNotExist))
			},
		}, {
			desc: "invalid path",
			key:  ".",
			expect: func(t *testing.T, _ fs.File, err error) {
				isInvalidPathError(t, err)
			},
		}, {
			desc: "invalid path",
			key:  "../invalid",
			expect: func(t *testing.T, _ fs.File, err error) {
				isInvalidPathError(t, err)
			},
		},
	}

	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			var api *mock.S3API
			if tcase.mock != nil {
				api = tcase.mock(t)
			}
			fsys := s3.NewBucketFS(api, tcase.bucket)
			f, err := fsys.OpenFile(ctx, tcase.key)
			tcase.expect(t, f, err)
		})
	}
}

func TestSeek_Mock(t *testing.T) {
	ctx := context.Background()
	content := []byte("Hello, World! This is test content for seeking.")
	obj := &mock.Object{
		Key:          "seekable-file.txt",
		Body:         content,
		LastModified: time.Now(),
	}

	type testCase struct {
		desc   string
		offset int64
		whence int
		expect func(*testing.T, fs.File, int64, error)
	}

	cases := []testCase{
		{
			desc:   "SeekStart to beginning",
			offset: 0,
			whence: io.SeekStart,
			expect: func(t *testing.T, f fs.File, pos int64, err error) {
				be.NilErr(t, err)
				be.Equal(t, int64(0), pos)
				buf := make([]byte, 5)
				n, err := f.Read(buf)
				be.NilErr(t, err)
				be.Equal(t, 5, n)
				be.Equal(t, "Hello", string(buf))
			},
		},
		{
			desc:   "SeekStart to middle",
			offset: 7,
			whence: io.SeekStart,
			expect: func(t *testing.T, f fs.File, pos int64, err error) {
				be.NilErr(t, err)
				be.Equal(t, int64(7), pos)
				buf := make([]byte, 5)
				n, err := f.Read(buf)
				be.NilErr(t, err)
				be.Equal(t, 5, n)
				be.Equal(t, "World", string(buf))
			},
		},
		{
			desc:   "SeekEnd negative offset",
			offset: -8,
			whence: io.SeekEnd,
			expect: func(t *testing.T, f fs.File, pos int64, err error) {
				be.NilErr(t, err)
				be.Equal(t, int64(len(content)-8), pos)
				buf := make([]byte, 8)
				n, err := f.Read(buf)
				be.NilErr(t, err)
				be.Equal(t, 8, n)
				be.Equal(t, "seeking.", string(buf))
			},
		},
		{
			desc:   "SeekStart negative offset error",
			offset: -1,
			whence: io.SeekStart,
			expect: func(t *testing.T, f fs.File, pos int64, err error) {
				be.Nonzero(t, err)
			},
		},
		{
			desc:   "SeekStart past EOF",
			offset: int64(len(content) + 10),
			whence: io.SeekStart,
			expect: func(t *testing.T, f fs.File, pos int64, err error) {
				be.NilErr(t, err)
				be.Equal(t, int64(len(content)+10), pos)
				// Read should return EOF
				buf := make([]byte, 5)
				_, readErr := f.Read(buf)
				be.True(t, errors.Is(readErr, io.EOF))
			},
		},
	}

	for i, tcase := range cases {
		t.Run(strconv.Itoa(i)+"-"+tcase.desc, func(t *testing.T) {
			api := mock.New(bucket, obj)
			fsys := s3.NewBucketFS(api, bucket)
			f, err := fsys.OpenFile(ctx, obj.Key)
			be.NilErr(t, err)
			defer f.Close()

			seeker, ok := f.(io.Seeker)
			be.True(t, ok)

			pos, err := seeker.Seek(tcase.offset, tcase.whence)
			tcase.expect(t, f, pos, err)
		})
	}
}

func TestSeekCurrent_Mock(t *testing.T) {
	ctx := context.Background()
	content := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	obj := &mock.Object{
		Key:          "alphabet.txt",
		Body:         content,
		LastModified: time.Now(),
	}

	api := mock.New(bucket, obj)
	fsys := s3.NewBucketFS(api, bucket)
	f, err := fsys.OpenFile(ctx, obj.Key)
	be.NilErr(t, err)
	defer f.Close()

	seeker := f.(io.Seeker)

	// Read first 5 bytes
	buf := make([]byte, 5)
	n, err := f.Read(buf)
	be.NilErr(t, err)
	be.Equal(t, 5, n)
	be.Equal(t, "ABCDE", string(buf))

	// Seek forward 5 from current (should be at position 10)
	pos, err := seeker.Seek(5, io.SeekCurrent)
	be.NilErr(t, err)
	be.Equal(t, int64(10), pos)

	// Read next 5 bytes
	n, err = f.Read(buf)
	be.NilErr(t, err)
	be.Equal(t, 5, n)
	be.Equal(t, "KLMNO", string(buf))

	// Seek backward from current
	pos, err = seeker.Seek(-10, io.SeekCurrent)
	be.NilErr(t, err)
	be.Equal(t, int64(5), pos)

	// Read next 5 bytes
	n, err = f.Read(buf)
	be.NilErr(t, err)
	be.Equal(t, 5, n)
	be.Equal(t, "FGHIJ", string(buf))
}

func TestSeekWithZip(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Skip("s3 test service is not running")
	}
	ctx := context.Background()

	// Create a zip file in memory
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	files := map[string]string{
		"hello.txt":      "Hello, World!",
		"dir/nested.txt": "Nested content",
		"numbers.txt":    "1234567890",
	}
	for name, content := range files {
		fw, err := zw.Create(name)
		be.NilErr(t, err)
		_, err = fw.Write([]byte(content))
		be.NilErr(t, err)
	}
	be.NilErr(t, zw.Close())

	// Upload zip to real S3
	fsys := testutil.TmpS3FS(t, nil)
	_, err := fsys.Write(ctx, "archive.zip", &zipBuf)
	be.NilErr(t, err)

	// Open the zip file from S3
	f, err := fsys.OpenFile(ctx, "archive.zip")
	be.NilErr(t, err)
	defer f.Close()

	// Get file info for size
	info, err := f.Stat()
	be.NilErr(t, err)

	// Create a ReaderAt from our ReadSeeker
	seeker := f.(io.ReadSeeker)
	readerAt := &seekerReaderAt{rs: seeker}

	// Open as zip archive - this requires seeking!
	zr, err := zip.NewReader(readerAt, info.Size())
	be.NilErr(t, err)

	// Verify we can read all files
	be.Equal(t, len(files), len(zr.File))

	for _, zf := range zr.File {
		expectedContent, exists := files[zf.Name]
		be.True(t, exists)

		rc, err := zf.Open()
		be.NilErr(t, err)

		content, err := io.ReadAll(rc)
		be.NilErr(t, err)
		rc.Close()

		be.Equal(t, expectedContent, string(content))
	}
}

// seekerReaderAt adapts an io.ReadSeeker to io.ReaderAt
type seekerReaderAt struct {
	rs io.ReadSeeker
}

func (s *seekerReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	_, err = s.rs.Seek(off, io.SeekStart)
	if err != nil {
		return 0, err
	}
	return s.rs.Read(p)
}

// TestOpenFileErrNotExist_Smithy404 covers one of the several shapes a
// missing key comes back as: HeadObject can return a *smithyhttp.ResponseError
// with status 404 rather than a typed types.NotFound. errIsNotExist must
// recognize it, so OpenFile still maps it to fs.ErrNotExist.
func TestOpenFileErrNotExist_Smithy404(t *testing.T) {
	ctx := context.Background()
	orig := smithy404Err()
	api := &headErrAPI{S3API: mock.New(bucket), headErr: orig}
	fsys := s3.NewBucketFS(api, bucket)
	_, err := fsys.OpenFile(ctx, "missing-key.txt")
	notExistWraps(t, "open", err, orig)
}

// TestOpenFileMissingKey_Integration runs against real S3 or an S3-compatible
// store (e.g. MinIO) when $OCFL_TEST_S3 is set and verifies that opening a
// missing key maps to fs.ErrNotExist regardless of the error shape the store
// returns (a smithy http response error on real S3, types.NoSuchKey on some
// S3-compatible stores, etc.).
func TestOpenFileMissingKey_Integration(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Skip("s3 test service is not running: set $OCFL_TEST_S3 to enable")
	}
	ctx := context.Background()
	fsys := testutil.TmpS3FS(t, nil)
	_, err := fsys.OpenFile(ctx, "missing-key.txt")
	isNotExistError(t, "open", err)
	_, err = fsys.Copy(ctx, "dst-file.txt", "missing-src.txt")
	isNotExistError(t, "copy", err)
}

// TestOpenFile_NilContentLengthError is the regression test for the
// openFile/s3File panic: openFile accepted a HEAD response with a nil
// ContentLength and returned an s3File, but Stat and Read dereference
// *f.info.ContentLength, so either call panicked on the nil pointer. The
// fix rejects the open with a "missing content length" error, so no s3File
// with an unknown size ever exists to Stat or Read.
func TestOpenFile_NilContentLengthError(t *testing.T) {
	ctx := context.Background()
	api := &nilLengthAPI{S3API: mock.New(bucket)}
	fsys := s3.NewBucketFS(api, bucket)

	f, err := fsys.OpenFile(ctx, "some-key.txt")
	if err == nil {
		// Pre-fix behavior: openFile accepted the object and returned an
		// s3File. Stat and Read dereference f.info.ContentLength, which is
		// nil here — the panic this regression guards. Exercise both on
		// the buggy file so the failure is the original nil-deref panic
		// rather than a silent acceptance of the unknown size.
		if f != nil {
			defer f.Close()
			if _, serr := f.Stat(); serr == nil {
				t.Error("Stat on a nil-ContentLength file returned nil error (nil-deref panic expected)")
			}
			if _, rerr := f.Read(make([]byte, 16)); rerr == nil {
				t.Error("Read on a nil-ContentLength file returned nil error (nil-deref panic expected)")
			}
		}
		t.Fatal(`OpenFile accepted an object with nil ContentLength; want a "missing content length" error`)
	}
	missingContentLengthErr(t, "open", "some-key.txt", err)
	be.Zero(t, f)
}
