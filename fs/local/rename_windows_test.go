//go:build windows

package local

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
)

// TestRenameReplaceWindowsOverwriteFile covers replacement of an existing
// destination file: the helper must replace dst with src (both the
// MoveFileEx path and, when MoveFileEx fails, the Remove+Rename fallback
// land here), and src must no longer exist afterwards. On the old
// os.Rename-based behavior this test fails with ERROR_ACCESS_DENIED.
func TestRenameReplaceWindowsOverwriteFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	be.NilErr(t, os.WriteFile(src, []byte("new contents"), 0644))
	be.NilErr(t, os.WriteFile(dst, []byte("old contents"), 0644))

	be.NilErr(t, renameReplaceWindows(src, dst))

	got, err := os.ReadFile(dst)
	be.NilErr(t, err)
	be.Equal(t, "new contents", string(got))
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Errorf("src still exists after replace (stat err = %v), want ErrNotExist", err)
	}
}

// TestRenameReplaceWindowsNonEmptyDirDestination covers failure when the
// destination is a non-empty directory: MoveFileEx cannot replace a
// directory with a file, the Remove fallback cannot remove a non-empty
// directory, so both strategies fail, src must be untouched, and the
// returned error must identify the MoveFileEx failure.
func TestRenameReplaceWindowsNonEmptyDirDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst")
	be.NilErr(t, os.WriteFile(src, []byte("contents"), 0644))
	be.NilErr(t, os.Mkdir(dst, 0755))
	be.NilErr(t, os.WriteFile(filepath.Join(dst, "child.txt"), []byte("child"), 0644))

	err := renameReplaceWindows(src, dst)
	be.Nonzero(t, err)
	if !strings.Contains(err.Error(), "MoveFileEx") {
		t.Errorf("error %q does not identify the MoveFileEx failure", err)
	}
	if _, statErr := os.Lstat(src); statErr != nil {
		t.Errorf("src was moved or removed despite failure: %v", statErr)
	}
}
