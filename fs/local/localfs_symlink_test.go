//go:build !windows

package local

// localfs_symlink_test.go pins mode handling in FS.Write when the target is a
// symlink. The atomic write (temp file + rename) replaces the target entry, so
// the question is which mode the replacement regular file gets.
//
// Neither mode available at the target is the right one to copy. os.Stat would
// follow the link and stamp the referent's permissions onto a file that is not
// the referent; the link's own mode (0777 on POSIX) would publish a
// world-writable regular file. Write therefore only preserves a mode when
// Lstat reports a regular file, so a symlinked target is written as a new
// file at the default temp mode. These tests pin that, and pin the POSIX
// rename behavior the scenario relies on: renaming a symlink over an existing
// regular file leaves a symlink at the destination.
//
// The file is POSIX-only (build tag !windows): Windows has no guaranteed,
// unprivileged symlink creation.
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
	// the default, so a leak from either direction is visible. The mode is
	// derived from the measured default rather than fixed: with a
	// restrictive umask (077), a hard-coded 0600 would *be* the default new
	// file mode and the leak check below would report a leak that did not
	// happen. XOR keeps the result inside rwx bits and never equals the
	// default, and the default is never the symlink's 0777 (both are 0666 &^
	// umask, and umask has no 0100 bit in any bit position common to them).
	referentMode := defaultPerm ^ 0o006
	if referentMode == 0 {
		// defaultPerm 0006: only possible under a umask like 0660. Fall back
		// to a fixed mode that is neither the default nor 0777.
		referentMode = 0o400
	}
	referent := filepath.Join(root, "referent")
	be.NilErr(t, os.WriteFile(referent, []byte("referent data"), referentMode))
	// WriteFile is umask-masked; chmod after so the referent holds exactly
	// the intended mode even when referentMode has bits the umask removes.
	be.NilErr(t, os.Chmod(referent, referentMode))

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
	if after.Mode().Perm() == referentMode {
		t.Fatalf("referent mode leaked onto the replacement: %v", after.Mode().Perm())
	}

	// The referent itself is untouched: the rename replaced the link entry,
	// and the link was never followed.
	refData, err := os.ReadFile(referent)
	be.NilErr(t, err)
	be.Equal(t, "referent data", string(refData))
	refInfo, err := os.Stat(referent)
	be.NilErr(t, err)
	be.Equal(t, referentMode, refInfo.Mode().Perm())
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

// The tests below pin containment: a symlink inside the storage root must
// never let a name reach a file outside it. Each one builds the same shape --
// an "outside" directory next to the root, a symlink in the root pointing at
// it -- and asserts both halves of the guarantee: the operation fails, and
// the external target is exactly as it was.
//
// They assert a non-nil error rather than a sentinel deliberately. os.Root
// reports an escape as an unexported errors.errorString ("path escapes from
// parent") that matches none of fs.ErrNotExist, fs.ErrInvalid or
// fs.ErrPermission, and the error a caller actually gets varies by method:
// writing through an intermediate symlink fails in MkdirAll with "file
// exists", not with the escape error at all. Pinning any particular error
// would pin an implementation detail of the standard library; the surviving
// external state is the property that matters.

// escapeFixture builds a root containing "link" -> outside, where outside
// holds a file "secret" and a directory "subdir" with a file in it. It
// returns the FS, the root path and the outside path.
func escapeFixture(t *testing.T) (*FS, string, string) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	be.NilErr(t, os.MkdirAll(root, 0o755))
	be.NilErr(t, os.MkdirAll(filepath.Join(outside, "subdir"), 0o755))
	be.NilErr(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("secret data"), 0o600))
	be.NilErr(t, os.WriteFile(filepath.Join(outside, "subdir", "nested"), []byte("nested data"), 0o600))
	// WriteFile is umask-masked; chmod so these assertions can rely on the
	// exact modes.
	be.NilErr(t, os.Chmod(filepath.Join(outside, "secret"), 0o600))
	be.NilErr(t, os.Chmod(filepath.Join(outside, "subdir", "nested"), 0o600))
	be.NilErr(t, os.Symlink(outside, filepath.Join(root, "link")))
	return MustNewFS(root), root, outside
}

// assertOutsideIntact checks that nothing under outside was created, removed
// or modified.
func assertOutsideIntact(t *testing.T, outside string) {
	t.Helper()
	secret, err := os.ReadFile(filepath.Join(outside, "secret"))
	be.NilErr(t, err)
	be.Equal(t, "secret data", string(secret))
	nested, err := os.ReadFile(filepath.Join(outside, "subdir", "nested"))
	be.NilErr(t, err)
	be.Equal(t, "nested data", string(nested))
	entries, err := os.ReadDir(outside)
	be.NilErr(t, err)
	be.Equal(t, 2, len(entries)) // secret, subdir -- nothing new
}

// TestFS_Write_IntermediateSymlinkEscape: Write("link/pwned.txt") created
// outside/pwned.txt before the FS was rebuilt on os.Root.
func TestFS_Write_IntermediateSymlinkEscape(t *testing.T) {
	fsys, _, outside := escapeFixture(t)
	_, err := fsys.Write(context.Background(), "link/pwned.txt", strings.NewReader("pwned"))
	be.True(t, err != nil)
	if _, statErr := os.Lstat(filepath.Join(outside, "pwned.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("write through an intermediate symlink created a file outside the root")
	}
	assertOutsideIntact(t, outside)
}

// TestFS_Remove_IntermediateSymlinkEscape: Remove("link/secret") deleted
// outside/secret before the rebuild.
func TestFS_Remove_IntermediateSymlinkEscape(t *testing.T) {
	fsys, _, outside := escapeFixture(t)
	err := fsys.Remove(context.Background(), "link/secret")
	be.True(t, err != nil)
	assertOutsideIntact(t, outside)
}

// TestFS_RemoveAll_IntermediateSymlinkEscape: RemoveAll("link/subdir")
// deleted outside/subdir and returned nil before the rebuild -- the worst of
// the four, since the caller saw success.
func TestFS_RemoveAll_IntermediateSymlinkEscape(t *testing.T) {
	fsys, _, outside := escapeFixture(t)
	err := fsys.RemoveAll(context.Background(), "link/subdir")
	be.True(t, err != nil)
	assertOutsideIntact(t, outside)
}

// TestFS_Read_IntermediateSymlinkEscape covers the read path, which os.DirFS
// documents as following symlinks out of the directory it was given.
func TestFS_Read_IntermediateSymlinkEscape(t *testing.T) {
	fsys, _, outside := escapeFixture(t)
	ctx := context.Background()

	t.Run("OpenFile", func(t *testing.T) {
		f, err := fsys.OpenFile(ctx, "link/secret")
		if err == nil {
			f.Close()
			t.Fatal("OpenFile read a file outside the storage root")
		}
	})

	t.Run("DirEntries", func(t *testing.T) {
		var (
			got     []string
			dirErr  error
			entries = fsys.DirEntries(ctx, "link")
		)
		for entry, err := range entries {
			if err != nil {
				dirErr = err
				continue
			}
			got = append(got, entry.Name())
		}
		be.True(t, dirErr != nil)
		be.Equal(t, 0, len(got))
	})

	// Reading the link itself is equally an escape: the target is outside.
	t.Run("OpenFile on the link", func(t *testing.T) {
		f, err := fsys.OpenFile(ctx, "link")
		if err == nil {
			f.Close()
			t.Fatal("OpenFile followed a symlink out of the storage root")
		}
	})

	assertOutsideIntact(t, outside)
}

// TestFS_Remove_SymlinkAtName pins the other half of removal: a symlink named
// directly is removed as the link entry it is, leaving its referent -- even an
// external one -- untouched. This is not an escape and must succeed.
func TestFS_Remove_SymlinkAtName(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remove func(*FS, context.Context) error
	}{
		{"Remove", func(fsys *FS, ctx context.Context) error { return fsys.Remove(ctx, "link") }},
		{"RemoveAll", func(fsys *FS, ctx context.Context) error { return fsys.RemoveAll(ctx, "link") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fsys, root, outside := escapeFixture(t)
			be.NilErr(t, tc.remove(fsys, context.Background()))
			if _, err := os.Lstat(filepath.Join(root, "link")); !os.IsNotExist(err) {
				t.Fatalf("link entry still present after %s", tc.name)
			}
			assertOutsideIntact(t, outside)
		})
	}
}

// TestFS_Write_SymlinkAtNameToExternalTarget covers the case the existing
// mode tests do not: the target name is a symlink whose referent is outside
// the root. The write must replace the link entry with a regular file inside
// the root and leave the external referent's contents and mode alone. It is
// not an escape -- the rename never follows the final component -- so it
// succeeds.
func TestFS_Write_SymlinkAtNameToExternalTarget(t *testing.T) {
	fsys, root, outside := escapeFixture(t)
	referent := filepath.Join(outside, "secret")
	be.NilErr(t, os.Symlink(referent, filepath.Join(root, "target")))

	n, err := fsys.Write(context.Background(), "target", strings.NewReader("replacement"))
	be.NilErr(t, err)
	be.Equal(t, int64(len("replacement")), n)

	// The name now holds a regular file inside the root with the new bytes.
	info, err := os.Lstat(filepath.Join(root, "target"))
	be.NilErr(t, err)
	if info.Mode()&fs.ModeSymlink != 0 {
		t.Fatal("write left a symlink at the target name")
	}
	data, err := os.ReadFile(filepath.Join(root, "target"))
	be.NilErr(t, err)
	be.Equal(t, "replacement", string(data))

	// The external referent kept both its contents and its mode: nothing was
	// written through the link, and its mode did not become the new file's.
	// The leak check compares against the *measured* default new-file mode:
	// under a restrictive umask (077) the default is 0600, the referent's
	// mode, and comparing modes directly would report a leak that did not
	// happen. The replacement is untainted when its mode is exactly the
	// default and the referent below is untouched.
	probe := "probe-mode"
	_, err = fsys.Write(context.Background(), probe, strings.NewReader("p"))
	be.NilErr(t, err)
	probeInfo, err := os.Lstat(filepath.Join(root, probe))
	be.NilErr(t, err)
	be.Equal(t, probeInfo.Mode().Perm(), info.Mode().Perm())

	refData, err := os.ReadFile(referent)
	be.NilErr(t, err)
	be.Equal(t, "secret data", string(refData))
	refInfo, err := os.Stat(referent)
	be.NilErr(t, err)
	be.Equal(t, fs.FileMode(0o600), refInfo.Mode().Perm())
	assertOutsideIntact(t, outside)
}

// TestFS_DirEntries_Sorted guards the trap in reading a directory handle
// directly: os.DirFS implements ReadDirFS and sorts internally, so the
// previous implementation got sorted output for free. os.File.ReadDir does
// not sort, so DirEntries sorts explicitly -- and this pins it, with names
// written in an order that is not the sorted one.
func TestFS_DirEntries_Sorted(t *testing.T) {
	fsys := MustNewFS(t.TempDir())
	ctx := context.Background()
	for _, name := range []string{"zulu", "alpha", "mike", "bravo", "delta"} {
		_, err := fsys.Write(ctx, "d/"+name, strings.NewReader(name))
		be.NilErr(t, err)
	}
	var got []string
	for entry, err := range fsys.DirEntries(ctx, "d") {
		be.NilErr(t, err)
		got = append(got, entry.Name())
	}
	want := []string{"alpha", "bravo", "delta", "mike", "zulu"}
	be.Equal(t, strings.Join(want, ","), strings.Join(got, ","))
}
