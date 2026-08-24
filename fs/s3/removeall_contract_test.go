package s3_test

import (
	"testing"

	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

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
