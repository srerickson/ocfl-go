package s3_test

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"testing"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

// removeRecorder wraps the standard mock.S3API and records the keys passed to
// HeadObject and DeleteObject so tests can assert the existence-check-then-
// delete sequence the WriteFS.Remove contract requires: a missing key must
// never reach the idempotent DeleteObject.
type removeRecorder struct {
	*mock.S3API
	headObjectCalls   []string
	deleteObjectCalls []string
}

var _ s3.RemoveAPI = (*removeRecorder)(nil)

func (r *removeRecorder) HeadObject(ctx context.Context, in *s3v2.HeadObjectInput, opts ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error) {
	if in.Key != nil {
		r.headObjectCalls = append(r.headObjectCalls, *in.Key)
	}
	return r.S3API.HeadObject(ctx, in, opts...)
}

func (r *removeRecorder) DeleteObject(ctx context.Context, in *s3v2.DeleteObjectInput, opts ...func(*s3v2.Options)) (*s3v2.DeleteObjectOutput, error) {
	if in.Key != nil {
		r.deleteObjectCalls = append(r.deleteObjectCalls, *in.Key)
	}
	return r.S3API.DeleteObject(ctx, in, opts...)
}

// TestRemove_MissingKey_ErrNotExist is the regression test for the bug where
// remove() deleted a missing key without checking existence first. S3's
// DeleteObject is idempotent — it succeeds (204) even for keys that do not
// exist — so Remove silently returned nil. The WriteFS.Remove contract
// (Option B, fs/fs.go) instead requires an error satisfying
// errors.Is(err, fs.ErrNotExist). The fix makes remove() HEAD-check existence
// before deleting: a missing key must surface as fs.ErrNotExist, and the
// idempotent DeleteObject must never be called for it.
func TestRemove_MissingKey_ErrNotExist(t *testing.T) {
	ctx := context.Background()
	rec := &removeRecorder{S3API: mock.New(bucket, &mock.Object{Key: "keep-me", Body: []byte("x")})}
	fsys := s3.NewBucketFS(rec, bucket)
	err := fsys.Remove(ctx, "no-such-key.txt")
	be.True(t, err != nil)
	var pathErr *fs.PathError
	be.True(t, errors.As(err, &pathErr))
	be.Equal(t, "remove", pathErr.Op)
	be.Equal(t, "no-such-key.txt", pathErr.Path)
	be.True(t, errors.Is(err, fs.ErrNotExist))
	// The existence probe ran, and the idempotent DeleteObject did not.
	be.AllEqual(t, []string{"no-such-key.txt"}, rec.headObjectCalls)
	be.Equal(t, 0, len(rec.deleteObjectCalls))
	// A failed remove of a missing key must not touch other objects.
	be.False(t, rec.Deleted["keep-me"])
}

// TestRemove_MissingKey_ErrorShapes verifies that a missing-key HEAD maps to
// fs.ErrNotExist for every error shape a real store can produce: the typed
// types.NotFound and types.NoSuchKey, real S3's *smithyhttp.ResponseError
// with status 404, and the *smithy.GenericAPIError carrying code "NoSuchKey"
// that some S3-compatible stores (e.g. MinIO) return.
func TestRemove_MissingKey_ErrorShapes(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		err  error
	}{
		{"types.NotFound", &types.NotFound{}},
		{"types.NoSuchKey", &types.NoSuchKey{}},
		{"smithy response error 404", smithy404Err()},
		{"smithy generic NoSuchKey", &smithy.GenericAPIError{Code: "NoSuchKey", Message: "m"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := &headErrAPI{S3API: mock.New(bucket), headErr: tc.err}
			fsys := s3.NewBucketFS(api, bucket)
			err := fsys.Remove(ctx, "missing-key.txt")
			be.True(t, errors.Is(err, fs.ErrNotExist))
			var pathErr *fs.PathError
			be.True(t, errors.As(err, &pathErr))
			be.Equal(t, "remove", pathErr.Op)
			be.Equal(t, "missing-key.txt", pathErr.Path)
			// The not-exist mapping must wrap, not replace: the store's
			// original error must remain reachable through the chain so
			// status/request-ID context survives for debugging, and the
			// direct PathError.Err must not be the sentinel itself.
			be.True(t, errors.Is(err, tc.err))
			be.True(t, pathErr.Err != fs.ErrNotExist)
		})
	}
}

// TestRemove_HeadErrorPreserved pins the other half of the existence-check
// behavior: a HEAD failure that is NOT a not-found (here a smithy http
// response error with status 500) must be returned as-is — never
// misclassified as fs.ErrNotExist — and DeleteObject must not run after a
// failed HEAD.
func TestRemove_HeadErrorPreserved(t *testing.T) {
	ctx := context.Background()
	headErr := &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusInternalServerError}},
		Err:      errors.New("head failed"),
	}
	api := &headErrAPI{S3API: mock.New(bucket, &mock.Object{Key: "keep-me", Body: []byte("x")}), headErr: headErr}
	fsys := s3.NewBucketFS(api, bucket)
	err := fsys.Remove(ctx, "some-key.txt")
	be.True(t, err != nil)
	be.False(t, errors.Is(err, fs.ErrNotExist))
	be.True(t, errors.Is(err, headErr))
	be.False(t, api.Deleted["some-key.txt"])
}

// TestRemove_Dot_ErrNotExist pins the documented "." behavior of the S3
// backend: removing the top-level directory is an error satisfying
// errors.Is(err, fs.ErrNotExist) (WriteFS.Remove docs, fs/fs.go), and it must
// issue no API calls at all, because the storage root is the bucket itself.
func TestRemove_Dot_ErrNotExist(t *testing.T) {
	ctx := context.Background()
	rec := &removeRecorder{S3API: mock.New(bucket, &mock.Object{Key: "keep-me", Body: []byte("x")})}
	fsys := s3.NewBucketFS(rec, bucket)
	err := fsys.Remove(ctx, ".")
	be.True(t, err != nil)
	var pathErr *fs.PathError
	be.True(t, errors.As(err, &pathErr))
	be.Equal(t, "remove", pathErr.Op)
	be.Equal(t, ".", pathErr.Path)
	be.True(t, errors.Is(err, fs.ErrNotExist))
	// Remove(".") is a pure guard: no existence probe, no delete, and no
	// object in the bucket is affected.
	be.Equal(t, 0, len(rec.headObjectCalls))
	be.Equal(t, 0, len(rec.deleteObjectCalls))
	be.False(t, rec.Deleted["keep-me"])
}

// TestWriteFSRemoveContract_S3 runs the shared WriteFS.Remove contract tests
// (internal/testutil) against the S3 backend with RemoveDotIsNotExist=true,
// matching the WriteFS.Remove documentation that the S3 backend reports
// fs.ErrNotExist for ".".
func TestWriteFSRemoveContract_S3(t *testing.T) {
	fsys := s3.NewBucketFS(mock.New(bucket), bucket)
	testutil.TestWriteFSRemoveContract(t, fsys, testutil.WriteFSRemoveContract{RemoveDotIsNotExist: true})
}

// TestRemove_Integration runs against real S3 or an S3-compatible store (e.g.
// MinIO) when $OCFL_TEST_S3 is set. It pins the Option B missing-key contract
// against a store whose DeleteObject silently succeeds (204) for missing
// keys, plus the "." guard and a clean removal of a present key.
func TestRemove_Integration(t *testing.T) {
	if !testutil.S3Enabled() {
		t.Skip("s3 test service is not running: set $OCFL_TEST_S3 to enable")
	}
	ctx := context.Background()
	fsys := testutil.TmpS3FS(t, nil)

	// Removing a missing key must surface as fs.ErrNotExist (option B), not
	// nil: on real S3 the DeleteObject-only path would silently succeed.
	err := fsys.Remove(ctx, "missing-key.txt")
	isNotExistError(t, "remove", err)

	// Seed the bucket, then Remove("."): the guard must report fs.ErrNotExist
	// and leave the seeded object readable (storage root unaffected).
	_, err = fsys.Write(ctx, "root-probe.txt", strings.NewReader("probe payload"))
	be.NilErr(t, err)
	err = fsys.Remove(ctx, ".")
	isNotExistError(t, "remove", err)
	file, err := fsys.OpenFile(ctx, "root-probe.txt")
	be.NilErr(t, err)
	be.NilErr(t, file.Close())

	// A present key still removes cleanly, after which it reads as missing.
	err = fsys.Remove(ctx, "root-probe.txt")
	be.NilErr(t, err)
	_, err = fsys.OpenFile(ctx, "root-probe.txt")
	isNotExistError(t, "open-after-remove", err)
}
