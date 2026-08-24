package local_test

import (
	"testing"

	"github.com/srerickson/ocfl-go/internal/testutil"
)

// TestWriteFSRemoveAllContract_Local runs the shared WriteFS.RemoveAll
// contract against the local backend, which refuses "." (its storage root
// must survive) and removes a file addressed directly, matching os.RemoveAll.
// The package-level ocflfs.RemoveAll(".") fallback that covers for the
// refusal is pinned in removeall_root_test.go.
func TestWriteFSRemoveAllContract_Local(t *testing.T) {
	fsys := testutil.TmpLocalFS(t)
	testutil.TestWriteFSRemoveAllContract(t, fsys, testutil.WriteFSRemoveAllContract{
		RemoveAllDotIsError:      true,
		RemoveAllOnFileRemovesIt: true,
	})
}
