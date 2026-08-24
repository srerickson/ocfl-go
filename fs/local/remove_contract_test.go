package local_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/fs/local"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

// TestWriteFSRemoveContract_Local runs the shared WriteFS.Remove contract
// tests (internal/testutil) against the local backend with
// RemoveDotIsNotExist=false: per the WriteFS.Remove docs (fs/fs.go), the
// local backend returns a descriptive *fs.PathError for "." rather than
// fs.ErrNotExist, while a missing file still satisfies
// errors.Is(err, fs.ErrNotExist).
func TestWriteFSRemoveContract_Local(t *testing.T) {
	fsys := testutil.TmpLocalFS(t)
	testutil.TestWriteFSRemoveContract(t, fsys, testutil.WriteFSRemoveContract{RemoveDotIsNotExist: false})
}

// TestRemove_MissingFile_ErrNotExist pins the Option B missing-file contract
// on the local backend: removing a file that does not exist returns an error
// satisfying errors.Is(err, fs.ErrNotExist) (underlying os.Remove error).
func TestRemove_MissingFile_ErrNotExist(t *testing.T) {
	fsys := testutil.TmpLocalFS(t)
	err := fsys.Remove(context.Background(), "no-such-file.txt")
	be.True(t, err != nil)
	var pathErr *fs.PathError
	be.True(t, errors.As(err, &pathErr))
	be.Equal(t, "remove", pathErr.Op)
	be.Equal(t, "no-such-file.txt", pathErr.Path)
	be.True(t, errors.Is(err, fs.ErrNotExist))
}

// TestRemove_Dot_PathError pins the documented local "." behavior: a
// descriptive *fs.PathError (deliberately NOT fs.ErrNotExist — "." is the one
// name the WriteFS.Remove contract lets be backend-specific), and the root
// directory still exists afterwards.
func TestRemove_Dot_PathError(t *testing.T) {
	dir := t.TempDir()
	fsys, err := local.NewFS(dir)
	be.NilErr(t, err)
	err = fsys.Remove(context.Background(), ".")
	be.True(t, err != nil)
	be.False(t, errors.Is(err, fs.ErrNotExist))
	var pathErr *fs.PathError
	be.True(t, errors.As(err, &pathErr))
	be.Equal(t, "remove", pathErr.Op)
	be.Equal(t, ".", pathErr.Path)
	be.Equal(t, "cannot remove top-level directory", pathErr.Err.Error())
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Fatalf("root directory was removed: %v", statErr)
	}
}
