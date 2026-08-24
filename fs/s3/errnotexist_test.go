package s3_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"testing"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

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
