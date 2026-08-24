//go:build !windows

package local

// localfs_symlink_test.go pins mode handling in FS.Write when the target is a
// symlink. The atomic write (temp file + rename) replaces the target entry, so
// the question is which mode the replacement regular file gets.
//
// Neither mode available at the target is the right one to copy. os.Stat would
// follow the link and stamp the referent's permissions onto a file that is not
// the referent; the link's own mode (0777 on POSIX) would publish a
// world-writable regular file. Write therefore uses Lstat to detect the
// symlink and then treats the write as a new file, taking the default temp
// mode. These tests pin that, and pin the POSIX rename behavior the scenario
// relies on: renaming a symlink over an existing regular file leaves a symlink
// at the destination.
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

// TestFS_Write_SymlinkTargetGetsDefaultMode walks the full scenario: create a
// symlink source, rename it over an existing regular file, assert the
// destination is still a symlink, then write through that symlinked target and
// assert the replacement regular file gets the default new-file mode — not the
// symlink's 0777 and not the referent's 0600.
func TestFS_Write_SymlinkTargetGetsDefaultMode(t *testing.T) {
	root := t.TempDir()
	fsys, err := NewFS(root)
	be.NilErr(t, err)

	// The mode a brand-new file gets in this directory, measured rather than
	// assumed: tempPerm is masked by the process umask, which the test does
	// not control.
	_, err = fsys.Write(context.Background(), "probe", strings.NewReader("probe"))
	be.NilErr(t, err)
	probeInfo, err := os.Lstat(filepath.Join(root, "probe"))
	be.NilErr(t, err)
	defaultPerm := probeInfo.Mode().Perm()

	// Referent with a mode that differs from both the symlink's own mode and
	// the default, so a leak from either direction is visible.
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
	if resolved, rerr := os.Readlink(dst); rerr != nil {
		t.Fatalf("readlink dst: %v", rerr)
	} else if resolved != referent {
		t.Fatalf("dst points at %q, want %q", resolved, referent)
	}

	// Write through the symlinked target. The atomic rename replaces the link
	// entry with the temp file, and no mode is preserved from the link.
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
	if after.Mode().Perm() != defaultPerm {
		t.Fatalf("replacement mode = %v, want the default new-file mode %v", after.Mode().Perm(), defaultPerm)
	}
	// Spelled out separately from the equality above: these are the two
	// specific leaks the Lstat-plus-skip logic exists to prevent, and a
	// change to defaultPerm must not quietly make either one acceptable.
	if after.Mode().Perm() == linkInfo.Mode().Perm() {
		t.Fatalf("symlink's own mode leaked onto the replacement: %v (world-writable on POSIX)", after.Mode().Perm())
	}
	if after.Mode().Perm() == 0o600 {
		t.Fatalf("referent mode leaked onto the replacement: %v", after.Mode().Perm())
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

// TestFS_Write_PreservesModeZero pins the one mode a "preserveMode != 0"
// sentinel cannot represent: an existing target with no permission bits set
// must keep 0000 rather than being silently widened to the default temp mode.
func TestFS_Write_PreservesModeZero(t *testing.T) {
	root := t.TempDir()
	fsys, err := NewFS(root)
	be.NilErr(t, err)
	ctx := context.Background()

	_, err = fsys.Write(ctx, "locked", strings.NewReader("first"))
	be.NilErr(t, err)
	target := filepath.Join(root, "locked")
	be.NilErr(t, os.Chmod(target, 0o000))

	// Writing goes through the temp file, so the unreadable/unwritable target
	// is never opened directly; only the rename touches it.
	_, err = fsys.Write(ctx, "locked", strings.NewReader("second"))
	be.NilErr(t, err)

	info, err := os.Lstat(target)
	be.NilErr(t, err)
	if info.Mode().Perm() != 0 {
		t.Fatalf("mode 0000 not preserved: got %v", info.Mode().Perm())
	}

	// Content still landed, checked after restoring read access.
	be.NilErr(t, os.Chmod(target, 0o600))
	data, err := os.ReadFile(target)
	be.NilErr(t, err)
	be.Equal(t, "second", string(data))
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
