package s3_test

import (
	"context"
	"testing"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

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
