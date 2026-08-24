package s3_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/carlmjohnson/be"

	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

// nilLengthAPI is a stub S3 API whose HeadObject always returns a response
// with a nil ContentLength, reproducing the S3-compatible stores and proxies
// that omit Content-Length on HEAD responses. Every other operation falls
// through to the in-memory mock, so these tests exercise the real
// implementation on both the copy and open paths.
type nilLengthAPI struct {
	*mock.S3API
}

func (a *nilLengthAPI) HeadObject(context.Context, *s3v2.HeadObjectInput, ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error) {
	return &s3v2.HeadObjectOutput{}, nil
}

// missingContentLengthErr asserts that err is the guard error the copy and
// open paths return for a missing HEAD ContentLength: a *fs.PathError with
// the given op/path and the "missing content length" message shared with
// MultiCopier.Copy.
func missingContentLengthErr(t *testing.T, op, path string, err error) {
	t.Helper()
	be.Nonzero(t, err)
	var perr *fs.PathError
	be.True(t, errors.As(err, &perr))
	be.Equal(t, op, perr.Op)
	be.Equal(t, path, perr.Path)
	be.Equal(t, "missing content length", perr.Err.Error())
}

// TestCopy_NilContentLengthError is the regression test for the copy()
// panic: copy() dereferenced *srcHead.ContentLength with no nil check when
// picking the copy strategy (single CopyObject vs multipart), so a HEAD
// response without Content-Length panicked with a nil pointer dereference.
// The fix mirrors the guard in MultiCopier.Copy and returns a "missing
// content length" error before any CopyObject or multipart machinery runs.
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
