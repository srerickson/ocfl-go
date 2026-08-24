//go:build !windows

package local

// POSIX-only tests for rename_posix.go: the symlink semantics of
// renameReplace — the primitive FS.Write calls for its final swap — where
// os.Rename moves link entries without following them:
//
//   - replacing an existing symlink target with a regular file replaces
//     the link entry itself, leaving the referent untouched, and
//   - renaming a symlink source over an existing regular file leaves a
//     symlink at the destination pointing at the same referent.
//
// The file is POSIX-only (build tag !windows): Windows has no guaranteed,
// unprivileged symlink creation, and Windows replace behavior is covered
// by rename_windows_test.go plus the cross-platform rename_test.go.
import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
)

// TestRenameReplaceOverwriteSymlinkTarget replaces an existing symlink at
// dst with a regular src file: the link entry is swapped out for the
// regular file (the referent is not followed, not modified, and not
// deleted), src no longer exists, and the regular file's content lands at
// dst. This is the "symlinked targets get silently converted to regular
// files" behavior the root issue called out — the conversion is the
// correct, documented replacement semantics (the write target entry is
// replaced), but the mode preservation must come from the link's own
// mode, which is pinned by TestFS_Write_ModePreservationUsesLstat.
func TestRenameReplaceOverwriteSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	referent := filepath.Join(dir, "referent")
	be.NilErr(t, os.WriteFile(referent, []byte("referent data"), 0o600))

	link := filepath.Join(dir, "link")
	be.NilErr(t, os.Symlink(referent, link))
	if info, err := os.Lstat(link); err != nil {
		t.Fatalf("lstat link: %v", err)
	} else if info.Mode()&fs.ModeSymlink == 0 {
		t.Fatalf("expected a symlink at %q, got mode %v", link, info.Mode())
	}

	src := filepath.Join(dir, "src.txt")
	be.NilErr(t, os.WriteFile(src, []byte("new contents"), 0o644))

	be.NilErr(t, renameReplace(src, link))

	// The destination is now a regular file with the new content.
	info, err := os.Lstat(link)
	be.NilErr(t, err)
	if info.Mode()&fs.ModeSymlink != 0 {
		t.Fatalf("replace must leave the regular file at %q, not the link", link)
	}
	got, err := os.ReadFile(link)
	be.NilErr(t, err)
	be.Equal(t, "new contents", string(got))
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Fatalf("src %q still exists after replace (stat err = %v), want ErrNotExist", src, err)
	}

	// The referent was never followed: it still exists, unchanged.
	refData, err := os.ReadFile(referent)
	be.NilErr(t, err)
	be.Equal(t, "referent data", string(refData))
}

// TestRenameReplaceSymlinkSource renames a symlink source over an
// existing regular file: the link entry itself is moved, so the
// destination becomes a symlink to the same referent (with the link's own
// mode), the source path is gone, and the referent is untouched. This is
// the POSIX rename behavior that the Lstat mode-preservation scenario in
// write_unix_test.go relies on, pinned through the renameReplace
// primitive rather than a raw os.Rename call.
func TestRenameReplaceSymlinkSource(t *testing.T) {
	dir := t.TempDir()
	referent := filepath.Join(dir, "referent")
	be.NilErr(t, os.WriteFile(referent, []byte("referent data"), 0o600))

	link := filepath.Join(dir, "src-link")
	be.NilErr(t, os.Symlink(referent, link))
	linkInfo, err := os.Lstat(link)
	be.NilErr(t, err)

	dst := filepath.Join(dir, "dst")
	be.NilErr(t, os.WriteFile(dst, []byte("old contents"), 0o644))

	be.NilErr(t, renameReplace(link, dst))

	// dst is still a symlink — the link entry was moved, not followed.
	dstInfo, err := os.Lstat(dst)
	be.NilErr(t, err)
	if dstInfo.Mode()&fs.ModeSymlink == 0 {
		t.Fatalf("renaming a symlink over an existing regular file must leave a symlink at the destination; got mode %v", dstInfo.Mode())
	}
	if resolved, rerr := os.Readlink(dst); rerr != nil {
		t.Fatalf("readlink dst: %v", rerr)
	} else if resolved != referent {
		t.Fatalf("dst points at %q, want %q", resolved, referent)
	}
	if dstInfo.Mode() != linkInfo.Mode() {
		t.Fatalf("destination symlink mode = %v, want source symlink mode %v", dstInfo.Mode(), linkInfo.Mode())
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("source link %q still exists after rename (stat err = %v), want ErrNotExist", link, err)
	}
	// The referent is untouched; the overwritten regular file's content is
	// only reachable through the link now, so it is gone.
	refData, err := os.ReadFile(referent)
	be.NilErr(t, err)
	be.Equal(t, "referent data", string(refData))
}
