package s3_test

import (
	"testing"

	"github.com/srerickson/ocfl-go/fs/s3"
	"github.com/srerickson/ocfl-go/fs/s3/internal/mock"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

// TestDirEntriesContract_S3 runs the shared DirEntriesFS contract against the
// S3 backend, using the in-process mock so it runs in CI without a store.
func TestDirEntriesContract_S3(t *testing.T) {
	fsys := s3.NewBucketFS(mock.New(bucket), bucket)
	testutil.TestDirEntriesContract(t, fsys)
}
