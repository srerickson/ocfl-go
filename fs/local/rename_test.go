package local

// rename_test.go covers the cross-platform rename-replace primitive
// (renameReplace) that FS.Write uses for its final swap, and the
// overwrite/mode-preservation behavior of FS.Write itself. The tests run
// on every platform: on POSIX they exercise the os.Rename path, on
// Windows the renameReplaceWindows path (MoveFileEx, then Remove+Rename).
//
// Symlink-specific rename cases live in rename_symlink_test.go (POSIX
// only — Windows has no guaranteed unprivileged symlink creation).
import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// TestFS_WriteOverwriteRegularFile covers overwriting an existing regular
// file through the full FS.Write path: the temp file is moved over the
// existing target, the content is replaced, and no temp files leak. On
// Windows this is the regression case for the rename helper — before it,
// the final os.Rename failed and the write returned an error.
func TestFS_WriteOverwriteRegularFile(t *testing.T) {
	root := t.TempDir()
	fsys, err := NewFS(root)
	be.NilErr(t, err)

	target := filepath.Join(root, "doc.txt")
	be.NilErr(t, os.WriteFile(target, []byte("old contents"), 0o644))

	n, err := fsys.Write(context.Background(), "doc.txt", strings.NewReader("new contents"))
	be.NilErr(t, err)
	be.Equal(t, int64(len("new contents")), n)

	got, err := os.ReadFile(target)
	be.NilErr(t, err)
	be.Equal(t, "new contents", string(got))
	be.Equal(t, 0, len(atomicTempFiles(t, root)))
}

// TestFS_WriteModePreservation pins that an overwrite keeps the existing
// target's permissions: Write must copy the old mode onto the temp file
// (chmod before the move) instead of leaving the default temp mode
// behind. The assertion is stability of the mode across the write, which
// holds on every platform: on POSIX the target is 0600 and the default
// temp mode would be 0644 &^ umask — an unpreserved write changes the
// reported mode and fails the test; on Windows 0600 is not representable
// (files are 0666 or 0444 read-only), so the equality assertion is the
// meaningful cross-platform statement, and exact mode bits are left to
// the POSIX-only symlink tests.
func TestFS_WriteModePreservation(t *testing.T) {
	root := t.TempDir()
	fsys, err := NewFS(root)
	be.NilErr(t, err)

	target := filepath.Join(root, "locked.txt")
	be.NilErr(t, os.WriteFile(target, []byte("old contents"), 0o600))
	before, err := os.Lstat(target)
	be.NilErr(t, err)

	if _, err := fsys.Write(context.Background(), "locked.txt", strings.NewReader("new contents")); err != nil {
		t.Fatalf("write over existing file: %v", err)
	}

	after, err := os.Lstat(target)
	be.NilErr(t, err)
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("mode after overwrite = %v, want preserved mode %v", after.Mode().Perm(), before.Mode().Perm())
	}
	got, err := os.ReadFile(target)
	be.NilErr(t, err)
	be.Equal(t, "new contents", string(got))
}
