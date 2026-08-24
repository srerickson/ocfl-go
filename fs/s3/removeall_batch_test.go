package s3_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"strconv"
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

func TestRemoveAll_Mock(t *testing.T) {
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
