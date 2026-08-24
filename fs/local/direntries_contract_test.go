package local_test

import (
	"testing"

	"github.com/srerickson/ocfl-go/internal/testutil"
)

// TestDirEntriesContract_Local runs the shared DirEntriesFS contract against
// the local backend.
func TestDirEntriesContract_Local(t *testing.T) {
	fsys := testutil.TmpLocalFS(t)
	testutil.TestDirEntriesContract(t, fsys)
}
