package s3_test

// Tests for remove.go: BucketFS.Remove, including the shared cross-backend
// Remove contract.

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

// TestRemove_MissingKey_ErrNotExist pins the WriteFS.Remove contract
// (fs/fs.go) on S3: removing a key that does not exist must return an error
// satisfying errors.Is(err, fs.ErrNotExist). S3 does not give that for free —
// DeleteObject is idempotent and succeeds (204) for a missing key — so
// remove() HEAD-checks existence first. The assertions cover both halves: the
// error surfaces as fs.ErrNotExist, and the idempotent DeleteObject is never
// called for a key that isn't there.
func TestRemove_MissingKey_ErrNotExist(t *testing.T) {
	ctx := context.Background()
	rec := mock.New(bucket, &mock.Object{Key: "keep-me", Body: []byte("x")})
	fsys := s3.NewBucketFS(rec, bucket)
	err := fsys.Remove(ctx, "no-such-key.txt")
	be.True(t, err != nil)
	var pathErr *fs.PathError
	be.True(t, errors.As(err, &pathErr))
	be.Equal(t, "remove", pathErr.Op)
	be.Equal(t, "no-such-key.txt", pathErr.Path)
	be.True(t, errors.Is(err, fs.ErrNotExist))
	// The existence probe ran, and the idempotent DeleteObject did not.
	be.AllEqual(t, []string{"no-such-key.txt"}, rec.KeysFor("HeadObject"))
	be.Equal(t, 0, len(rec.KeysFor("DeleteObject")))
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
	rec := mock.New(bucket, &mock.Object{Key: "keep-me", Body: []byte("x")})
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
	be.Equal(t, 0, len(rec.KeysFor("HeadObject")))
	be.Equal(t, 0, len(rec.KeysFor("DeleteObject")))
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
// MinIO) when $OCFL_TEST_S3 is set. It pins the missing-key fs.ErrNotExist
// contract against a real store whose DeleteObject silently succeeds (204)
// for missing keys, plus the "." guard and a clean removal of a present key.
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

func TestRemove(t *testing.T) {
	ctx := context.Background()
	type testCase struct {
		desc   string
		bucket string
		key    string
		mock   func(*testing.T) *mock.S3API
		expect func(*testing.T, *mock.S3API, error)
	}
	cases := []testCase{
		{
			desc: "invalid path",
			key:  "../file.txt",
			expect: func(t *testing.T, _ *mock.S3API, err error) {
				isInvalidPathError(t, err)
			},
		}, {
			desc:   "remove file",
			bucket: bucket,
			key:    "remove-me",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, &mock.Object{Key: "remove-me"}, &mock.Object{Key: "keep-me"})
			},
			expect: func(t *testing.T, state *mock.S3API, err error) {
				be.NilErr(t, err)
				be.True(t, state.Deleted["remove-me"])
				be.False(t, state.Deleted["keep-me"])
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
			err := fsys.Remove(ctx, tcase.key)
			tcase.expect(t, api, err)
		})
	}
}
