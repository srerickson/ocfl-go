package local_test

import (
	"context"
	"strings"
	"testing"

	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

// TestWalkFilesContract_Local runs the shared WalkFiles contract tests
// (internal/testutil) against the local backend. The local FS does not
// implement the FileWalker optimization (it has no WalkFiles method), so the
// package-level ocflfs.WalkFiles walks it through the DirEntries-based
// fileWalk path — the same generic walk every non-optimized backend gets.
//
// The error fixture makes the walk of "blocked" fail by planting a regular
// file where the walk expects to descend into a directory: the directory
// read then errors, which fileWalk must deliver as the pair's error element
// and stop on.
func TestWalkFilesContract_Local(t *testing.T) {
	fsys := testutil.TmpLocalFS(t)
	testutil.TestWalkFilesContract(t, fsys, testutil.WalkFilesContract{
		ErrWalk: func(t *testing.T) ocflfs.WriteFS {
			errFS := testutil.TmpLocalFS(t)
			if _, err := errFS.Write(context.Background(), "blocked", strings.NewReader("x")); err != nil {
				t.Fatalf("seeding blocked file: %v", err)
			}
			return errFS
		},
	})
}
