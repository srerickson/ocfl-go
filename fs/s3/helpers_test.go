package s3_test

// Shared fixtures, fakes and assertion helpers for the s3_test package.
//
// A helper belongs here when more than one test file uses it; one needed by a
// single file stays in that file. Keeping the shared ones together means a
// helper's home does not depend on the order the tests happened to be
// written in.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"net/http"
	"path/filepath"
	"testing"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

const (
	bucket   = "ocfl-go-test"
	megabyte = 1024 * 1024
	partSize = 6 * megabyte
)

// fixtures is the on-disk content-fixture directory used to seed test buckets.
var fixtures = filepath.Join("..", "..", "testdata", "content-fixture")

func isInvalidPathError(t *testing.T, err error) {
	t.Helper()
	isPathError(t, err)
	if !errors.Is(err, fs.ErrInvalid) {
		t.Error("error is not fs.ErrInvalid")
	}
}

func isPathError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Error("expected non-nil error")
		return
	}
	var pErr *fs.PathError
	if !errors.As(err, &pErr) {
		t.Error("error is not fs.PathError")
	}
}

func compareFileInf(t *testing.T, info, fixture fs.FileInfo) {
	t.Helper()
	be.Equal(t, fixture.Name(), info.Name())
	be.Equal(t, fixture.IsDir(), info.IsDir())
	if !fixture.IsDir() {
		be.Equal(t, fixture.Size(), info.Size())
	}
}

func comparDirEntries(
	t *testing.T,
	entries iter.Seq2[fs.DirEntry, error],
	fixtures iter.Seq2[fs.DirEntry, error],
) {
	t.Helper()
	nextFixture2, stop := iter.Pull2(fixtures)
	defer stop()
	for entry, err := range entries {
		fixtureEntry, fixtureErr, ok := nextFixture2()
		be.True(t, ok)
		be.Equal(t, fixtureErr, err)
		if err != nil {
			be.Zero(t, entry)
			continue
		}
		be.Equal(t, fixtureEntry.Name(), entry.Name())
		be.Equal(t, fixtureEntry.IsDir(), entry.IsDir())
		fixtureInfo, err := fixtureEntry.Info()
		be.NilErr(t, err)
		entryInfo, err := entry.Info()
		be.NilErr(t, err)
		compareFileInf(t, fixtureInfo, entryInfo)
	}
	// no more fixture entries
	_, _, more := nextFixture2()
	be.False(t, more)
}

// headErrAPI is a *mock.S3API whose HeadObject method always returns headErr.
// It mimics the behavior of real S3 (and some S3-compatible stores) where
// HeadObject on a missing key returns a generic smithy http response error
// with status 404 rather than a typed types.NotFound or types.NoSuchKey,
// because HEAD responses have no body from which to deserialize an error
// shape.
type headErrAPI struct {
	*mock.S3API
	headErr error
}

func (a *headErrAPI) HeadObject(context.Context, *s3v2.HeadObjectInput, ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error) {
	return nil, a.headErr
}

// smithy404Err returns the error shape produced by HeadObject on a missing key
// with real S3: a *smithyhttp.ResponseError with HTTP status 404 wrapping the
// failed error deserialization. The wrapped error is intentionally not a
// types.NotFound or types.NoSuchKey, since the regression is exactly that
// errIsNotExist() doesn't recognize this error shape.
func smithy404Err() error {
	return &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusNotFound}},
		Err: &smithy.DeserializationError{
			Err: fmt.Errorf("no response body to deserialize into error shape"),
		},
	}
}

// isNotExistError asserts that err is a *fs.PathError wrapping fs.ErrNotExist
// and logs the concrete error type for diagnostics.
func isNotExistError(t *testing.T, op string, err error) {
	t.Helper()
	isPathError(t, err)
	t.Logf("%s: underlying error type: %T", op, err)
	be.True(t, errors.Is(err, fs.ErrNotExist))
}

// notExistWraps is the strengthened form of isNotExistError: it asserts the
// not-exist mapping satisfies the fs.ErrNotExist contract, and that the
// original backend error survives in the chain instead of being replaced.
// The replacement pattern (fsErr.Err = fs.ErrNotExist) discarded the
// smithy/HTTP error with its status code and request ID, which is the
// regression this task removes.
func notExistWraps(t *testing.T, op string, err error, orig error) {
	t.Helper()
	isNotExistError(t, op, err)
	// The original error must remain reachable through the unwrap chain
	// (e.g. for errors.As or errors.Is by a caller debugging against
	// MinIO or real S3).
	be.True(t, errors.Is(err, orig))
	// The direct PathError.Err must be a wrapper around the sentinel, not
	// the sentinel substituted in place of the original error.
	var pathErr *fs.PathError
	be.True(t, errors.As(err, &pathErr))
	be.True(t, pathErr.Err != fs.ErrNotExist)
}

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
