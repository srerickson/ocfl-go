package local

// Tests for the rename-replace primitive FS.Write uses for its final swap:
// rename_posix.go on POSIX, rename_windows.go on Windows. The tests here run
// on every platform; the symlink cases are POSIX-only and live in
// rename_unix_test.go, the MoveFileEx cases in rename_windows_test.go.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
)

// TestRenameReplaceOverwriteRegularFile pins the core replacement
// contract shared by both renameReplace implementations: an existing
// regular file at dst is replaced by src (content and all), and src no
// longer exists afterwards. This is the operation that fails on Windows
// with a plain os.Rename (ERROR_ACCESS_DENIED / ERROR_ALREADY_EXISTS on
// some Go and filesystem combinations) and motivated the Windows helper.
func TestRenameReplaceOverwriteRegularFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	be.NilErr(t, os.WriteFile(src, []byte("new contents"), 0o644))
	be.NilErr(t, os.WriteFile(dst, []byte("old contents"), 0o644))

	be.NilErr(t, renameReplace(src, dst))

	got, err := os.ReadFile(dst)
	be.NilErr(t, err)
	be.Equal(t, "new contents", string(got))
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Fatalf("src %q still exists after replace (stat err = %v), want ErrNotExist", src, err)
	}
}
