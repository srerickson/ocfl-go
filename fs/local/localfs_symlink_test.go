//go:build !windows

package local

// localfs_symlink_test.go pins mode preservation in FS.Write to Lstat
// semantics. The atomic write (temp file + rename) replaces the target
// entry, so when the target is a symlink the question is which mode the
// replacement inherits. os.Stat would follow the link and stamp the
// referent's permissions onto the new regular file; Lstat must be used so
// the symlink's own mode is preserved instead. The test also pins the
// POSIX rename behavior the scenario relies on: renaming a symlink over an
// existing regular file leaves a symlink at the destination.
//
// The file is POSIX-only (build tag !windows): Windows has no guaranteed,
// unprivileged symlink creation, and the Windows rename-over-existing path
// is covered by rename_windows_test.go and the integration task that wires
// the helper into Write.
import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
)

// TestFS_Write_ModePreservationUsesLstat walks the exact scenario from the
// spec: create a symlink source, rename it over an existing regular file,
// assert the destination is still a symlink with the expected symlink mode,
// then write through that symlinked target and assert the replacement
// regular file inherits the symlink's own mode (Lstat), never the
// referent's (Stat).
func TestFS_Write_ModePreservationUsesLstat(t *testing.T) {
	root := t.TempDir()
	fsys, err := NewFS(root)
	be.NilErr(t, err)

	// Referent with a mode that differs from the symlink's own mode, so the
	// Lstat behavior (symlink mode, typically 0777 on POSIX) is
	// distinguishable from the Stat behavior (referent mode, 0600 here).
	referent := filepath.Join(root, "referent")
	be.NilErr(t, os.WriteFile(referent, []byte("referent data"), 0o600))

	// A symlink source...
	link := filepath.Join(root, "src-link")
	be.NilErr(t, os.Symlink(referent, link))
	linkInfo, err := os.Lstat(link)
	be.NilErr(t, err)
	be.True(t, linkInfo.Mode()&fs.ModeSymlink != 0)

	// ...renamed over an existing regular file. POSIX rename moves the link
	// entry itself: the destination is still a symlink with the link's own
	// mode, not the replaced regular file's mode.
	dst := filepath.Join(root, "dst")
	be.NilErr(t, os.WriteFile(dst, []byte("old contents"), 0o644))
	be.NilErr(t, os.Rename(link, dst))

	dstInfo, err := os.Lstat(dst)
	be.NilErr(t, err)
	if dstInfo.Mode()&fs.ModeSymlink == 0 {
		t.Fatalf("renaming a symlink over an existing regular file must leave a symlink at the destination; got mode %v", dstInfo.Mode())
	}
	if dstInfo.Mode() != linkInfo.Mode() {
		t.Fatalf("destination symlink mode = %v, want source symlink mode %v", dstInfo.Mode(), linkInfo.Mode())
	}
	if resolved, rerr := os.Readlink(dst); rerr != nil {
		t.Fatalf("readlink dst: %v", rerr)
	} else if resolved != referent {
		t.Fatalf("dst points at %q, want %q", resolved, referent)
	}

	// Write through the symlinked target. The atomic rename replaces the
	// link entry with the temp file; mode preservation must come from Lstat
	// (the symlink's own mode), so the referent's 0600 is not stamped onto
	// the new regular file.
	if _, err := fsys.Write(context.Background(), "dst", strings.NewReader("new contents")); err != nil {
		t.Fatalf("write over symlinked target: %v", err)
	}

	data, err := os.ReadFile(dst)
	be.NilErr(t, err)
	be.Equal(t, "new contents", string(data))

	after, err := os.Lstat(dst)
	be.NilErr(t, err)
	if after.Mode()&fs.ModeSymlink != 0 {
		t.Fatalf("rename replace must leave the regular temp file at dst, not the link")
	}
	if after.Mode().Perm() != linkInfo.Mode().Perm() {
		t.Fatalf("replacement mode = %v, want the symlink's own mode %v (Lstat), not the referent's 0600 (Stat)",
			after.Mode(), linkInfo.Mode())
	}
	if after.Mode().Perm() == 0o600 {
		t.Fatalf("referent mode leaked onto the replacement: %v", after.Mode())
	}

	// The referent itself is untouched: the rename replaced the link entry,
	// and the link was never followed.
	refData, err := os.ReadFile(referent)
	be.NilErr(t, err)
	be.Equal(t, "referent data", string(refData))
	refInfo, err := os.Stat(referent)
	be.NilErr(t, err)
	be.Equal(t, fs.FileMode(0o600), refInfo.Mode().Perm())
}

// TestFS_Write_ToleratesLstatError pins the graceful error handling of the
// Lstat in Write's mode-preservation step: when the target does not exist
// (the common case), the write must proceed as a new file instead of
// failing on the stat itself.
func TestFS_Write_ToleratesLstatError(t *testing.T) {
	root := t.TempDir()
	fsys, err := NewFS(root)
	be.NilErr(t, err)

	if _, err := fsys.Write(context.Background(), "brand-new.bin", strings.NewReader("x")); err != nil {
		t.Fatalf("write to missing target: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "brand-new.bin"))
	be.NilErr(t, err)
	be.Equal(t, "x", string(data))
}
