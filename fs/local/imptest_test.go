package local_test

import (
	"context"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/internal/imptest"
	"github.com/srerickson/ocfl-go/fs/local"
)

// The fs implementation test suite is run from package local_test, not
// package local: imptest is free to import backend packages, so a backend may
// not import it back.

// tmpLocalFS returns a local.FS rooted in a fresh temp directory.
func tmpLocalFS(t *testing.T) *local.FS {
	t.Helper()
	fsys, err := local.NewFS(t.TempDir())
	be.NilErr(t, err)
	return fsys
}

// TestWriteFSWrite_Local runs the shared WriteFS.Write suite against the local
// backend.
func TestWriteFSWrite_Local(t *testing.T) {
	imptest.TestWriteFSWrite(t, tmpLocalFS(t))
}

// TestWriteFSRemove_Local runs the shared WriteFS.Remove suite against the
// local backend.
func TestWriteFSRemove_Local(t *testing.T) {
	imptest.TestWriteFSRemove(t, tmpLocalFS(t))
}

// TestWriteFSRemoveAll_Local runs the shared WriteFS.RemoveAll suite against
// the local backend, which refuses "." (its storage root must survive) and
// removes a file addressed directly, matching os.RemoveAll.
func TestWriteFSRemoveAll_Local(t *testing.T) {
	imptest.TestWriteFSRemoveAll(t, tmpLocalFS(t), imptest.WriteFSRemoveAll{
		RemoveAllDotIsError:      true,
		RemoveAllOnFileRemovesIt: true,
	})
}

// TestDirEntries_Local runs the shared DirEntriesFS suite against the local
// backend.
func TestDirEntries_Local(t *testing.T) {
	imptest.TestDirEntries(t, tmpLocalFS(t))
}

// TestWalkFiles_Local runs the shared WalkFiles suite against the local
// backend. local.FS has no WalkFiles method, so the package-level
// ocflfs.WalkFiles walks it through the DirEntries-based fileWalk — the same
// generic walk every non-optimized backend gets.
//
// The error fixture makes the walk of "blocked" fail by planting a regular
// file where the walk expects to descend into a directory, so the directory
// read errors.
func TestWalkFiles_Local(t *testing.T) {
	imptest.TestWalkFiles(t, tmpLocalFS(t), imptest.WalkFiles{
		ErrWalk: func(t *testing.T) ocflfs.WriteFS {
			errFS := tmpLocalFS(t)
			_, err := errFS.Write(context.Background(), "blocked", strings.NewReader("x"))
			be.NilErr(t, err)
			return errFS
		},
	})
}
