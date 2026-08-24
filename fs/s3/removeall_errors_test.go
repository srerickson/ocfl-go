package s3_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
)

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
