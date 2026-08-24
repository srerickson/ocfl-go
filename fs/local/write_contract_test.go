package local_test

import (
	"testing"

	"github.com/srerickson/ocfl-go/internal/testutil"
)

// TestWriteFSWriteContract_Local runs the shared WriteFS.Write contract
// against the local backend. The backend-specific assertions (atomicity as
// seen by a concurrent reader, temp-file placement, mode preservation) live
// in atomic_write_test.go and localfs_symlink_test.go.
func TestWriteFSWriteContract_Local(t *testing.T) {
	fsys := testutil.TmpLocalFS(t)
	testutil.TestWriteFSWriteContract(t, fsys, testutil.WriteFSWriteContract{
		WriteDotIsError: true,
	})
}
