package s3_test

// Tests for removeall.go: BucketFS.RemoveAll and RemoveRoot, including the
// shared cross-backend RemoveAll contract.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

// removeAllKey returns the i-th (zero-based) key under the "remove-all" prefix
// used by the batch tests. Keys are zero-padded so that the mock's
// lexicographic ordering matches numeric ordering.
func removeAllKey(i int) string {
	return fmt.Sprintf("remove-all/obj-%06d", i)
}

// removeAllKeys returns keys [start, start+n) in ascending order.
func removeAllKeys(start, n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = removeAllKey(start + i)
	}
	return keys
}

// batchRemoveAll issues RemoveAll over a bucket with n keys under the
// "remove-all" prefix and returns the recording mock for assertions.
func batchRemoveAll(t *testing.T, n int) *mock.S3API {
	t.Helper()
	objects := make([]*mock.Object, n)
	for i := range objects {
		objects[i] = &mock.Object{Key: removeAllKey(i)}
	}
	rec := mock.New(bucket, objects...)
	fsys := s3.NewBucketFS(rec, bucket)
	err := fsys.RemoveAll(context.Background(), "remove-all")
	be.NilErr(t, err)
	return rec
}

// TestRemoveAll_BatchSinglePage: a single page of <=1000 keys must be deleted
// with exactly one DeleteObjects call that carries every key. No per-key
// DeleteObject calls may be made.
func TestRemoveAll_BatchSinglePage(t *testing.T) {
	const n = 1000
	rec := batchRemoveAll(t, n)
	be.DeepEqual(t, [][]string{removeAllKeys(0, n)}, rec.KeyBatchesFor("DeleteObjects"))
	be.Equal(t, 0, rec.CallCount("DeleteObject"))
	for _, key := range removeAllKeys(0, n) {
		be.True(t, rec.Deleted[key])
	}
}

// TestRemoveAll_BatchMultiPage: 2500 keys span three list pages (1000, 1000,
// 500) and must produce exactly three DeleteObjects calls with 1000, 1000 and
// 500 keys respectively. No per-key DeleteObject calls may be made.
func TestRemoveAll_BatchMultiPage(t *testing.T) {
	const n = 2500
	rec := batchRemoveAll(t, n)
	be.Equal(t, 3, len(rec.KeyBatchesFor("DeleteObjects")))
	for i, pageSize := range []int{1000, 1000, 500} {
		be.DeepEqual(t, removeAllKeys(i*1000, pageSize), rec.KeyBatchesFor("DeleteObjects")[i])
	}
	be.Equal(t, 0, rec.CallCount("DeleteObject"))
	for _, key := range removeAllKeys(0, n) {
		be.True(t, rec.Deleted[key])
	}
}

// TestRemoveAll_BatchNoKeys: removing under a prefix with no objects must not
// issue any DeleteObjects or DeleteObject calls.
func TestRemoveAll_BatchNoKeys(t *testing.T) {
	rec := mock.New(bucket)
	fsys := s3.NewBucketFS(rec, bucket)
	err := fsys.RemoveAll(context.Background(), "remove-all")
	be.NilErr(t, err)
	be.Equal(t, 0, len(rec.KeyBatchesFor("DeleteObjects")))
	be.Equal(t, 0, rec.CallCount("DeleteObject"))
}

func TestRemoveAll(t *testing.T) {
	ctx := context.Background()
	type testCase struct {
		desc   string
		bucket string
		dir    string
		mock   func(*testing.T) *mock.S3API
		expect func(*testing.T, *mock.S3API, error)
	}
	cases := []testCase{
		{
			desc: "invalid path",
			dir:  "..",
			expect: func(t *testing.T, _ *mock.S3API, err error) {
				isInvalidPathError(t, err)
			},
		}, {
			desc:   "remove dir",
			bucket: bucket,
			dir:    "remove-me",
			mock: func(t *testing.T) *mock.S3API {
				return mock.New(bucket, &mock.Object{Key: "remove-me/file"}, &mock.Object{Key: "keep-me"})
			},
			expect: func(t *testing.T, state *mock.S3API, err error) {
				be.NilErr(t, err)
				be.True(t, state.Deleted["remove-me/file"])
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
			err := fsys.RemoveAll(ctx, tcase.dir)
			tcase.expect(t, api, err)
		})
	}

}

// deleteObjectsFailer wraps the standard mock.S3API and rewrites the
// DeleteObjects response to simulate a PARTIAL batch-delete failure. Real S3
// returns HTTP 200 for DeleteObjects even when individual keys in the batch
// fail (e.g. AccessDenied from a bucket policy); the failures arrive in the
// response body's Errors list while the successful keys land in Deleted. The
// wrapped mock's per-key state (m.Deleted) is updated to match, so the tests
// can assert that only the keys that "succeeded" were deleted.
type deleteObjectsFailer struct {
	*mock.S3API
	failKeys map[string]string // key -> failure message
}

var _ s3.RemoveAllAPI = (*deleteObjectsFailer)(nil)

func (f *deleteObjectsFailer) DeleteObjects(ctx context.Context, in *s3v2.DeleteObjectsInput, opts ...func(*s3v2.Options)) (*s3v2.DeleteObjectsOutput, error) {
	out := &s3v2.DeleteObjectsOutput{}
	for _, obj := range in.Delete.Objects {
		if obj.Key == nil {
			continue
		}
		if msg, ok := f.failKeys[*obj.Key]; ok {
			out.Errors = append(out.Errors, types.Error{
				Key:     obj.Key,
				Code:    aws.String("AccessDenied"),
				Message: aws.String(msg),
			})
			continue
		}
		out.Deleted = append(out.Deleted, types.DeletedObject{Key: obj.Key})
		f.Deleted[*obj.Key] = true
	}
	return out, nil
}

// TestRemoveAll_DeleteObjectsPartialFailure covers a batch DeleteObjects
// response that is HTTP 200 but carries a non-empty Errors list (one key
// denied by a bucket policy, one deleted). RemoveAll must surface a non-nil
// *fs.PathError whose message names the failed key instead of silently
// returning nil while the denied object survives.
func TestRemoveAll_DeleteObjectsPartialFailure(t *testing.T) {
	const (
		okKey     = "remove-all/ok-000001"
		deniedKey = "remove-all/denied-000002"
	)
	api := &deleteObjectsFailer{
		S3API: mock.New(bucket,
			&mock.Object{Key: okKey},
			&mock.Object{Key: deniedKey},
		),
		failKeys: map[string]string{deniedKey: "Access Denied"},
	}
	fsys := s3.NewBucketFS(api, bucket)
	// (4) must not panic
	err := fsys.RemoveAll(context.Background(), "remove-all")
	// (1) the returned error is non-nil
	if err == nil {
		t.Fatal("expected non-nil error from RemoveAll when DeleteObjects reports partial failure")
	}
	// (2) it is a *fs.PathError
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("expected *fs.PathError, got %T: %v", err, err)
	}
	// (3) the error string mentions the failed key
	if !strings.Contains(err.Error(), deniedKey) {
		t.Errorf("error %q does not mention failed key %q", err, deniedKey)
	}
	// the successful key must still have been deleted; the denied key must not
	be.True(t, api.Deleted[okKey])
	be.False(t, api.Deleted[deniedKey])
}

// TestRemoveAll_DeleteObjectsSuccess: when the batch DeleteObjects response
// carries no Errors, RemoveAll returns nil and every listed key is deleted.
func TestRemoveAll_DeleteObjectsSuccess(t *testing.T) {
	const (
		key1 = "remove-all/obj-000001"
		key2 = "remove-all/obj-000002"
	)
	api := &deleteObjectsFailer{
		S3API:    mock.New(bucket, &mock.Object{Key: key1}, &mock.Object{Key: key2}),
		failKeys: nil, // no per-key failures
	}
	fsys := s3.NewBucketFS(api, bucket)
	be.NilErr(t, fsys.RemoveAll(context.Background(), "remove-all"))
	be.True(t, api.Deleted[key1])
	be.True(t, api.Deleted[key2])
}

// TestRemoveAll_Dot_UsesBatchPath pins that ocflfs.RemoveAll(ctx, fsys, ".")
// dispatches through BucketFS.RemoveRoot (ocflfs.RootRemover) to the batched
// removeAll: one bucket-wide listing and a single batched DeleteObjects
// carrying every object, with no per-key DeleteObject calls — regardless of
// how deeply keys are nested. Without the RootRemover dispatch the generic
// per-entry fallback would run instead, deleting each top-level key with its
// own DeleteObject.
func TestRemoveAll_Dot_UsesBatchPath(t *testing.T) {
	keys := []string{"one.txt", "three/more/nested.txt", "two/deep.txt"}
	objects := make([]*mock.Object, len(keys))
	for i, key := range keys {
		// Insert in non-sorted order; the mock must still list them
		// sorted, matching a real bucket listing.
		objects[len(keys)-1-i] = &mock.Object{Key: key}
	}
	rec := mock.New(bucket, objects...)
	fsys := s3.NewBucketFS(rec, bucket)

	err := ocflfs.RemoveAll(context.Background(), fsys, ".")
	be.NilErr(t, err)

	// Exactly one batch DeleteObjects covering every key in the bucket (the
	// nested keys prove the listing was bucket-wide, not prefix-scoped)...
	be.DeepEqual(t, [][]string{{"one.txt", "three/more/nested.txt", "two/deep.txt"}}, rec.KeyBatchesFor("DeleteObjects"))
	// ...and no per-key DeleteObject calls at all.
	be.Equal(t, 0, rec.CallCount("DeleteObject"))
	for _, key := range keys {
		be.True(t, rec.Deleted[key])
	}
}

// TestWriteFSRemoveAllContract_S3 runs the shared WriteFS.RemoveAll contract
// against the S3 backend, using the in-process mock so it runs in CI without
// a store. S3 has no directories: "." empties the bucket, and a name is a key
// prefix, so RemoveAll on a file's own path matches nothing.
func TestWriteFSRemoveAllContract_S3(t *testing.T) {
	fsys := s3.NewBucketFS(mock.New(bucket), bucket)
	testutil.TestWriteFSRemoveAllContract(t, fsys, testutil.WriteFSRemoveAllContract{
		RemoveAllDotIsError:      false,
		RemoveAllOnFileRemovesIt: false,
	})
}
