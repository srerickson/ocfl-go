package s3_test

import (
	"context"
	"testing"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

// removeAllDotRecorder wraps the standard mock.S3API and records the keys
// passed to every DeleteObjects (batch) and DeleteObject (per-key) call, so a
// test can assert that generic fs.RemoveAll(".") reaches the backend's
// batched removeAll instead of degrading to per-key deletes.
type removeAllDotRecorder struct {
	*mock.S3API
	deleteObjectsCalls [][]string // keys per DeleteObjects call, in order
	deleteObjectCalls  []string   // keys per DeleteObject call, in order
}

var (
	_ s3.RemoveAllAPI = (*removeAllDotRecorder)(nil)
	_ s3.S3API        = (*removeAllDotRecorder)(nil)
)

func (r *removeAllDotRecorder) DeleteObjects(ctx context.Context, in *s3v2.DeleteObjectsInput, opts ...func(*s3v2.Options)) (*s3v2.DeleteObjectsOutput, error) {
	keys := make([]string, 0, len(in.Delete.Objects))
	for _, obj := range in.Delete.Objects {
		if obj.Key != nil {
			keys = append(keys, *obj.Key)
		}
	}
	r.deleteObjectsCalls = append(r.deleteObjectsCalls, keys)
	return r.S3API.DeleteObjects(ctx, in, opts...)
}

func (r *removeAllDotRecorder) DeleteObject(ctx context.Context, in *s3v2.DeleteObjectInput, opts ...func(*s3v2.Options)) (*s3v2.DeleteObjectOutput, error) {
	if in.Key != nil {
		r.deleteObjectCalls = append(r.deleteObjectCalls, *in.Key)
	}
	return r.S3API.DeleteObject(ctx, in, opts...)
}

// TestRemoveAll_Dot_UsesBatchPath pins that generic fs.RemoveAll(ctx, fsys,
// ".") reaches the S3 backend's batched removeAll: one bucket-wide listing
// and a single batched DeleteObjects carrying every object, with no per-key
// DeleteObject calls — regardless of how deeply keys are nested. The old
// fs.RemoveAll never delegated "." to the backend; it walked DirEntries(".")
// and deleted each top-level key individually (a per-key DeleteObject), so
// the backend's batch path was bypassed and this test fails before the fix.
func TestRemoveAll_Dot_UsesBatchPath(t *testing.T) {
	keys := []string{"one.txt", "three/more/nested.txt", "two/deep.txt"}
	objects := make([]*mock.Object, len(keys))
	for i, key := range keys {
		// Insert in non-sorted order; the mock must still list them
		// sorted, matching a real bucket listing.
		objects[len(keys)-1-i] = &mock.Object{Key: key}
	}
	rec := &removeAllDotRecorder{S3API: mock.New(bucket, objects...)}
	fsys := s3.NewBucketFS(rec, bucket)

	err := ocflfs.RemoveAll(context.Background(), fsys, ".")
	be.NilErr(t, err)

	// Exactly one batch DeleteObjects covering every key in the bucket (the
	// nested keys prove the listing was bucket-wide, not prefix-scoped)...
	be.DeepEqual(t, [][]string{{"one.txt", "three/more/nested.txt", "two/deep.txt"}}, rec.deleteObjectsCalls)
	// ...and no per-key DeleteObject calls at all.
	be.Equal(t, 0, len(rec.deleteObjectCalls))
	for _, key := range keys {
		be.True(t, rec.Deleted[key])
	}
}
