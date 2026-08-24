package local_test

// Tests for remove.go: FS.Remove and FS.RemoveAll, including the shared
// cross-backend Remove and RemoveAll contracts and the RemoveAll(".")
// dispatch that ocflfs.RemoveAll performs against this backend.

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/local"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

func TestFS_Remove(t *testing.T) {
	t.Run("removes existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()

		// Create a file
		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("content"))
		be.NilErr(t, err)

		// Remove it
		err = fsys.Remove(ctx, "test.txt")
		be.NilErr(t, err)

		// Verify it's gone
		_, err = os.Stat(filepath.Join(tmpDir, "test.txt"))
		be.True(t, os.IsNotExist(err))
	})

	t.Run("errors on non-existent file", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		err = fsys.Remove(ctx, "nonexistent.txt")
		be.True(t, err != nil)

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "remove", pathErr.Op)
	})

	t.Run("prevents removing root directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		err = fsys.Remove(ctx, ".")
		be.True(t, err != nil)
		// The name == "." guard returns a PathError naming the top-level
		// directory instead of calling os.Remove. Pin the guard's exact return
		// value and the safety property that the root itself still exists.

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "remove", pathErr.Op)
		be.Equal(t, ".", pathErr.Path)
		be.Equal(t, "cannot remove top-level directory", pathErr.Err.Error())
		if _, statErr := os.Stat(tmpDir); statErr != nil {
			t.Fatalf("root directory was removed: %v", statErr)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)

		// A canceled context must abort before any OS call: create a file,
		// cancel, and verify Remove reports the cancellation as a PathError
		// wrapping context.Canceled without touching the file.
		ctx, cancel := context.WithCancel(context.Background())
		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("content"))
		be.NilErr(t, err)
		cancel()

		err = fsys.Remove(ctx, "test.txt")
		be.True(t, errors.Is(err, context.Canceled))

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "remove", pathErr.Op)
		be.Equal(t, "test.txt", pathErr.Path)
		if _, statErr := os.Stat(filepath.Join(tmpDir, "test.txt")); statErr != nil {
			t.Fatalf("file was removed despite canceled context: %v", statErr)
		}
	})

	t.Run("rejects invalid path", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		// Traversal ("..") and other malformed names must be rejected with
		// fs.ErrInvalid before any OS call: osPath/fs.ValidPath blocks them,
		// pinning the security property that Remove can never escape the root.
		invalidPaths := []string{
			"../escape",
			"../outside",
			"/absolute",
			"./file",
		}

		for _, path := range invalidPaths {
			err = fsys.Remove(ctx, path)
			be.True(t, errors.Is(err, fs.ErrInvalid))

			var pathErr *fs.PathError
			be.True(t, errors.As(err, &pathErr))
			be.Equal(t, "remove", pathErr.Op)
			be.Equal(t, path, pathErr.Path)
		}
	})
}

func TestFS_RemoveAll(t *testing.T) {
	t.Run("removes directory tree", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()

		// Create nested files
		_, err = fsys.Write(ctx, "dir/file1.txt", strings.NewReader("content1"))
		be.NilErr(t, err)
		_, err = fsys.Write(ctx, "dir/subdir/file2.txt", strings.NewReader("content2"))
		be.NilErr(t, err)

		// Remove entire directory tree
		err = fsys.RemoveAll(ctx, "dir")
		be.NilErr(t, err)

		// Verify it's gone
		_, err = os.Stat(filepath.Join(tmpDir, "dir"))
		be.True(t, os.IsNotExist(err))
	})

	t.Run("removes single file", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()

		// Create a file
		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("content"))
		be.NilErr(t, err)

		// Remove it with RemoveAll
		err = fsys.RemoveAll(ctx, "test.txt")
		be.NilErr(t, err)

		// Verify it's gone
		_, err = os.Stat(filepath.Join(tmpDir, "test.txt"))
		be.True(t, os.IsNotExist(err))
	})

	t.Run("removes symlink without following it outside the root", func(t *testing.T) {
		// Regression pin for RemoveAll symlink safety. A symlink at a path
		// inside the storage root must be removed as a link and never
		// followed to its target — even when that target is a directory
		// outside the root. Production RemoveAll drives removal through
		// os.Root.RemoveAll (see the comment in RemoveAll), which unlinks
		// the final component without following it, so the link itself is
		// what is removed and nothing outside the storage root is touched.
		// Any rewrite that follows the link — with or without a trailing
		// slash on the OS path — fails this test by deleting the external
		// target.)
		root := t.TempDir()
		fsys, err := local.NewFS(root)
		be.NilErr(t, err)

		// External target: a sibling temporary directory outside the
		// storage root, containing a known file.
		ext := filepath.Join(t.TempDir(), "external")
		be.NilErr(t, os.MkdirAll(ext, 0o755))
		victim := filepath.Join(ext, "victim.txt")
		be.NilErr(t, os.WriteFile(victim, []byte("precious"), 0o644))

		// A symlink inside the storage root pointing at the external
		// directory.
		link := filepath.Join(root, "link")
		be.NilErr(t, os.Symlink(ext, link))

		be.NilErr(t, fsys.RemoveAll(context.Background(), "link"))

		// The symlink entry itself is removed...
		if _, statErr := os.Lstat(link); !os.IsNotExist(statErr) {
			t.Fatalf("symlink still present after RemoveAll: %v", statErr)
		}
		// ...and the external target directory and its file are untouched:
		// following the link would have deleted them.
		info, statErr := os.Stat(ext)
		be.NilErr(t, statErr)
		be.True(t, info.IsDir())
		data, readErr := os.ReadFile(victim)
		be.NilErr(t, readErr)
		be.Equal(t, "precious", string(data))
	})

	t.Run("rejects intermediate symlink escaping the root", func(t *testing.T) {
		// Regression pin for RemoveAll symlink safety through INTERMEDIATE
		// path components. os.RemoveAll opens the parent directory by full
		// path (OpenFile(parentDir) in os.removeAll), so it FOLLOWS a
		// symlink at an intermediate component: with name "link/subdir"
		// and root/link -> external, the parent "link" resolves to the
		// external directory and RemoveAll silently deletes ext/subdir
		// (err == nil) while leaving root/link in place. RemoveAll must
		// instead walk every component with openat-based operations that
		// validate symlink targets stay within the root (os.Root), so the
		// escape is refused with an error and the external target survives. A symlink at the FINAL component
		// remains safe (pinned by "removes symlink without following it
		// outside the root" above): it is unlinked as a link, never
		// descended into.
		root := t.TempDir()
		fsys, err := local.NewFS(root)
		be.NilErr(t, err)

		// External target outside the storage root, with known content.
		ext := filepath.Join(t.TempDir(), "external")
		be.NilErr(t, os.MkdirAll(filepath.Join(ext, "subdir"), 0o755))
		victim := filepath.Join(ext, "subdir", "inner.txt")
		be.NilErr(t, os.WriteFile(victim, []byte("precious"), 0o644))

		// A symlink inside the storage root pointing at the external
		// directory. "link/subdir" is a valid name per fs.ValidPath, so
		// the escape is not blocked by name validation; it happens when
		// the intermediate "link" component is resolved.
		link := filepath.Join(root, "link")
		be.NilErr(t, os.Symlink(ext, link))

		err = fsys.RemoveAll(context.Background(), "link/subdir")
		be.True(t, err != nil)

		// The refusal is reported as a PathError with the operation and
		// name used by every other local FS error.
		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "remove", pathErr.Op)
		be.Equal(t, "link/subdir", pathErr.Path)

		// The external target and its contents survive: the pre-fix
		// behavior deleted them and returned nil.
		if _, statErr := os.Stat(ext); statErr != nil {
			t.Fatalf("external dir missing after RemoveAll: %v", statErr)
		}
		data, readErr := os.ReadFile(victim)
		be.NilErr(t, readErr)
		be.Equal(t, "precious", string(data))
		// The planted symlink is also untouched (the removal was refused
		// before anything under it was reached).
		if _, statErr := os.Lstat(link); statErr != nil {
			t.Fatalf("symlink missing after refused RemoveAll: %v", statErr)
		}
	})

	t.Run("follows intermediate symlink staying inside the root", func(t *testing.T) {
		// The symlink guard must not over-reject: an intermediate symlink
		// whose relative target resolves INSIDE the storage root is followed
		// (os.Root semantics), so a storage root that legitimately uses
		// relative in-root symlinks keeps working. Only escapes — an
		// absolute target, or a relative target resolving outside the
		// root — are refused.
		root := t.TempDir()
		fsys, err := local.NewFS(root)
		be.NilErr(t, err)

		_, err = fsys.Write(context.Background(), "real/sub/inner.txt", strings.NewReader("content"))
		be.NilErr(t, err)
		_, err = fsys.Write(context.Background(), "real/keep.txt", strings.NewReader("keep"))
		be.NilErr(t, err)

		// Relative in-root symlink: root/alias -> real.
		alias := filepath.Join(root, "alias")
		be.NilErr(t, os.Symlink("real", alias))

		be.NilErr(t, fsys.RemoveAll(context.Background(), "alias/sub"))

		// The in-root target path was removed...
		if _, statErr := os.Stat(filepath.Join(root, "real", "sub")); !os.IsNotExist(statErr) {
			t.Fatalf("in-root target not removed through in-root symlink: %v", statErr)
		}
		// ...the link itself survived (only the referenced path was
		// removed, the link entry is a separate top-level entry)...
		if _, statErr := os.Lstat(alias); statErr != nil {
			t.Fatalf("symlink removed along with target: %v", statErr)
		}
		// ...and the rest of the in-root target is untouched.
		if _, statErr := os.Stat(filepath.Join(root, "real", "keep.txt")); statErr != nil {
			t.Fatalf("unrelated in-root file removed: %v", statErr)
		}
	})

	t.Run("prevents removing root directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		err = fsys.RemoveAll(ctx, ".")
		be.True(t, err != nil)
		// The name == "." guard returns a PathError naming the top-level
		// directory instead of calling os.RemoveAll. Pin the guard's exact
		// return value and the safety property that the root itself still
		// exists.

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, ".", pathErr.Path)
		be.Equal(t, "remove", pathErr.Op)
		be.Equal(t, "cannot remove top-level directory", pathErr.Err.Error())
		if _, statErr := os.Stat(tmpDir); statErr != nil {
			t.Fatalf("root directory was removed: %v", statErr)
		}
	})

	t.Run("succeeds on non-existent path", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		// WriteFS contract: RemoveAll on a missing path returns nil, matching
		// os.RemoveAll. A missing path nested under an existing directory is
		// also a no-op that leaves the sibling file untouched.
		err = fsys.RemoveAll(ctx, "nonexistent")
		be.NilErr(t, err)

		_, err = fsys.Write(ctx, "dir/keep.txt", strings.NewReader("keep"))
		be.NilErr(t, err)
		err = fsys.RemoveAll(ctx, "dir/missing")
		be.NilErr(t, err)
		if _, statErr := os.Stat(filepath.Join(tmpDir, "dir", "keep.txt")); statErr != nil {
			t.Fatalf("existing file removed by missing-path RemoveAll: %v", statErr)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)

		// A canceled context must abort before any OS call: create a tree,
		// cancel, and verify RemoveAll reports the cancellation as a PathError
		// wrapping context.Canceled without touching the tree.
		ctx, cancel := context.WithCancel(context.Background())
		_, err = fsys.Write(ctx, "test/file.txt", strings.NewReader("content"))
		be.NilErr(t, err)
		cancel()

		err = fsys.RemoveAll(ctx, "test")
		be.True(t, errors.Is(err, context.Canceled))

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "remove", pathErr.Op)
		be.Equal(t, "test", pathErr.Path)
		if _, statErr := os.Stat(filepath.Join(tmpDir, "test", "file.txt")); statErr != nil {
			t.Fatalf("tree removed despite canceled context: %v", statErr)
		}
	})

	t.Run("rejects invalid path", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		// Traversal ("..") and other malformed names must be rejected with
		// fs.ErrInvalid before any OS call: osPath/fs.ValidPath blocks them,
		// pinning the security property that RemoveAll can never escape the
		// root.
		invalidPaths := []string{
			"../escape",
			"../outside",
			"/absolute",
			"./file",
		}

		for _, path := range invalidPaths {
			err = fsys.RemoveAll(ctx, path)
			be.True(t, errors.Is(err, fs.ErrInvalid))

			var pathErr *fs.PathError
			be.True(t, errors.As(err, &pathErr))
			be.Equal(t, "remove", pathErr.Op)
			be.Equal(t, path, pathErr.Path)
		}
	})
}

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

// TestRemove_MissingFile_ErrNotExist pins the WriteFS.Remove missing-file
// contract on the local backend: removing a file that does not exist returns
// an error satisfying errors.Is(err, fs.ErrNotExist) (the underlying
// os.Remove error).
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

// TestWriteFSRemoveAllContract_Local runs the shared WriteFS.RemoveAll
// contract against the local backend, which refuses "." (its storage root
// must survive) and removes a file addressed directly, matching os.RemoveAll.
// The package-level ocflfs.RemoveAll(".") fallback that covers for the
// refusal is pinned by TestBackendRemoveAllDotRefuses below.
func TestWriteFSRemoveAllContract_Local(t *testing.T) {
	fsys := testutil.TmpLocalFS(t)
	testutil.TestWriteFSRemoveAllContract(t, fsys, testutil.WriteFSRemoveAllContract{
		RemoveAllDotIsError:      true,
		RemoveAllOnFileRemovesIt: true,
	})
}

// TestRemoveAllDotUsesFallbackWalk pins the local backend's half of the
// ocflfs.RemoveAll(".") contract.
//
// The local storage root must survive, so *local.FS deliberately does not
// implement ocflfs.RootRemover: ocflfs.RemoveAll(".") takes the generic
// per-entry walk instead, emptying the root while leaving the directory
// itself in place. The backend's own RemoveAll(".") still refuses outright,
// which is what makes the opt-in-by-type dispatch necessary — an error there
// cannot be distinguished from a mid-operation failure.
func TestRemoveAllDotUsesFallbackWalk(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fsys, err := local.NewFS(root)
	be.NilErr(t, err)

	// The local backend must not advertise the bulk-root capability.
	if _, ok := any(fsys).(ocflfs.RootRemover); ok {
		t.Fatal("*local.FS must not implement ocflfs.RootRemover: its storage root must survive")
	}

	for _, name := range []string{"top.txt", "sub/nested.txt", "sub/deeper/leaf.txt"} {
		_, err := fsys.Write(ctx, name, strings.NewReader("x"))
		be.NilErr(t, err)
	}

	be.NilErr(t, ocflfs.RemoveAll(ctx, fsys, "."))

	// The root directory itself survives, and is now empty.
	info, err := os.Stat(root)
	be.NilErr(t, err)
	be.True(t, info.IsDir())
	remaining, err := os.ReadDir(root)
	be.NilErr(t, err)
	be.Equal(t, 0, len(remaining))

	// Calling it again on the emptied root is a no-op, not an error.
	be.NilErr(t, ocflfs.RemoveAll(ctx, fsys, "."))
}

// TestBackendRemoveAllDotRefuses pins that the backend method itself still
// refuses "." outright, without touching the storage root.
func TestBackendRemoveAllDotRefuses(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fsys, err := local.NewFS(root)
	be.NilErr(t, err)
	_, err = fsys.Write(ctx, "keep.txt", strings.NewReader("payload"))
	be.NilErr(t, err)

	err = fsys.RemoveAll(ctx, ".")
	be.Nonzero(t, err)
	var pathErr *fs.PathError
	be.True(t, errors.As(err, &pathErr))
	be.Equal(t, ".", pathErr.Path)

	// The root and its contents are untouched.
	data, err := os.ReadFile(filepath.Join(root, "keep.txt"))
	be.NilErr(t, err)
	be.Equal(t, "payload", string(data))
}
