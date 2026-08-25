package local_test

import (
	"context"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/internal/testutil"
	"github.com/srerickson/ocfl-go/fs/local"
)

// The contract callers live in package local_test, not package local: the
// shared suite is free to import backend packages, so a backend may not
// import it back.

// tmpLocalFS returns a local.FS rooted in a fresh temp directory.
func tmpLocalFS(t *testing.T) *local.FS {
	t.Helper()
	fsys, err := local.NewFS(t.TempDir())
	be.NilErr(t, err)
	return fsys
}

// TestWriteFSWriteContract_Local runs the shared WriteFS.Write contract
// against the local backend.
func TestWriteFSWriteContract_Local(t *testing.T) {
	testutil.TestWriteFSWriteContract(t, tmpLocalFS(t), testutil.WriteFSWriteContract{
		WriteDotIsError: true,
		// Write opens the destination with O_TRUNC and copies into it, so a
		// source that fails partway leaves the target truncated — and creates
		// an empty file where there was none.
		SkipFailedSourceKeepsFile: "local Write is not atomic: a failing source truncates or creates the target; see #163",
	})
}

// TestWriteFSRemoveContract_Local runs the shared WriteFS.Remove contract
// against the local backend. RemoveDotIsNotExist is false: the local backend
// returns a descriptive *fs.PathError for "." rather than fs.ErrNotExist,
// while a missing file still satisfies errors.Is(err, fs.ErrNotExist).
func TestWriteFSRemoveContract_Local(t *testing.T) {
	testutil.TestWriteFSRemoveContract(t, tmpLocalFS(t), testutil.WriteFSRemoveContract{
		RemoveDotIsNotExist: false,
	})
}

// TestWriteFSRemoveAllContract_Local runs the shared WriteFS.RemoveAll
// contract against the local backend, which refuses "." (its storage root
// must survive) and removes a file addressed directly, matching os.RemoveAll.
func TestWriteFSRemoveAllContract_Local(t *testing.T) {
	testutil.TestWriteFSRemoveAllContract(t, tmpLocalFS(t), testutil.WriteFSRemoveAllContract{
		RemoveAllDotIsError:      true,
		RemoveAllOnFileRemovesIt: true,
	})
}

// TestDirEntriesContract_Local runs the shared DirEntriesFS contract against
// the local backend.
func TestDirEntriesContract_Local(t *testing.T) {
	testutil.TestDirEntriesContract(t, tmpLocalFS(t))
}

// TestWalkFilesContract_Local runs the shared WalkFiles contract against the
// local backend. local.FS has no WalkFiles method, so the package-level
// ocflfs.WalkFiles walks it through the DirEntries-based fileWalk — the same
// generic walk every non-optimized backend gets.
//
// The error fixture makes the walk of "blocked" fail by planting a regular
// file where the walk expects to descend into a directory, so the directory
// read errors.
func TestWalkFilesContract_Local(t *testing.T) {
	testutil.TestWalkFilesContract(t, tmpLocalFS(t), testutil.WalkFilesContract{
		ErrWalk: func(t *testing.T) ocflfs.WriteFS {
			errFS := tmpLocalFS(t)
			_, err := errFS.Write(context.Background(), "blocked", strings.NewReader("x"))
			be.NilErr(t, err)
			return errFS
		},
		// fileWalk yields the directory-read error and then falls through to
		// e.Name() on the nil entry it was yielded with, so the walk panics
		// instead of terminating.
		SkipWalkErrors: "fileWalk panics on an error-yielding DirEntries; see #165",
	})
}
