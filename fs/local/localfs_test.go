package local

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

func TestNewFS(t *testing.T) {
	t.Run("creates FS with absolute path", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)
		be.Nonzero(t, fsys)

		root := fsys.Root()
		be.True(t, filepath.IsAbs(root))
	})

	t.Run("converts relative path to absolute", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Chdir(tmpDir)
		fsys, err := NewFS(".")
		be.NilErr(t, err)
		root := fsys.Root()
		be.True(t, filepath.IsAbs(root))
		be.True(t, strings.HasSuffix(root, filepath.Base(tmpDir)))
	})
}

func TestFS_Root(t *testing.T) {
	tmpDir := t.TempDir()
	fsys, err := NewFS(tmpDir)
	be.NilErr(t, err)

	root := fsys.Root()
	absPath, err := filepath.Abs(tmpDir)
	be.NilErr(t, err)
	be.Equal(t, absPath, root)
}

func TestFS_Write(t *testing.T) {
	t.Run("writes file successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		content := "test content"
		n, err := fsys.Write(ctx, "test.txt", strings.NewReader(content))
		be.NilErr(t, err)
		be.Equal(t, int64(len(content)), n)

		// Verify file was written
		data, err := os.ReadFile(filepath.Join(tmpDir, "test.txt"))
		be.NilErr(t, err)
		be.Equal(t, content, string(data))
	})

	t.Run("creates parent directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		content := "nested file"
		n, err := fsys.Write(ctx, "a/b/c/test.txt", strings.NewReader(content))
		be.NilErr(t, err)
		be.Equal(t, int64(len(content)), n)

		// Verify file was written in nested path
		data, err := os.ReadFile(filepath.Join(tmpDir, "a/b/c/test.txt"))
		be.NilErr(t, err)
		be.Equal(t, content, string(data))
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()

		// Write initial content
		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("first"))
		be.NilErr(t, err)

		// Overwrite with new content
		content := "second"
		n, err := fsys.Write(ctx, "test.txt", strings.NewReader(content))
		be.NilErr(t, err)
		be.Equal(t, int64(len(content)), n)

		// Verify new content
		data, err := os.ReadFile(filepath.Join(tmpDir, "test.txt"))
		be.NilErr(t, err)
		be.Equal(t, content, string(data))
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		// Canceled before Write: Write must return a *fs.PathError wrapping
		// context.Canceled without touching the filesystem.
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("content"))
		be.True(t, errors.Is(err, context.Canceled))

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "write", pathErr.Op)
		be.Equal(t, "test.txt", pathErr.Path)
	})

	t.Run("respects deadline exceeded", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		// Already-expired deadline: Write must return a *fs.PathError wrapping
		// context.DeadlineExceeded without touching the filesystem.
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("content"))
		be.True(t, errors.Is(err, context.DeadlineExceeded))

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "write", pathErr.Op)
		be.Equal(t, "test.txt", pathErr.Path)
	})

	t.Run("rejects invalid path", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()

		// Test various invalid paths. Traversal via ".." (e.g. "../escape")
		// must be rejected with fs.ErrInvalid before any file is touched:
		// osPath/fs.ValidPath blocks it, pinning the security property that
		// Write can never escape the FS root.
		invalidPaths := []string{
			"../escape",
			"../outside",
			"/absolute",
			"./file",
		}

		for _, path := range invalidPaths {
			_, err = fsys.Write(ctx, path, strings.NewReader("content"))
			be.True(t, errors.Is(err, fs.ErrInvalid))

			var pathErr *fs.PathError
			be.True(t, errors.As(err, &pathErr))
			be.Equal(t, "write", pathErr.Op)
			be.Equal(t, path, pathErr.Path)
		}
	})

	t.Run("sets correct file permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("content"))
		be.NilErr(t, err)

		// Check file permissions
		info, err := os.Stat(filepath.Join(tmpDir, "test.txt"))
		be.NilErr(t, err)
		be.Equal(t, fs.FileMode(0644), info.Mode().Perm())
	})
}

func TestFS_Remove(t *testing.T) {
	t.Run("removes existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
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
		fsys, err := NewFS(tmpDir)
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
		fsys, err := NewFS(tmpDir)
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
		fsys, err := NewFS(tmpDir)
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
		fsys, err := NewFS(tmpDir)
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
		fsys, err := NewFS(tmpDir)
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
		fsys, err := NewFS(tmpDir)
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

	t.Run("prevents removing root directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
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
		fsys, err := NewFS(tmpDir)
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
		fsys, err := NewFS(tmpDir)
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
		fsys, err := NewFS(tmpDir)
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

func TestFS_osPath(t *testing.T) {
	t.Run("converts valid fs path to OS path", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		osPath, err := fsys.osPath("test.txt")
		be.NilErr(t, err)
		be.Equal(t, filepath.Join(tmpDir, "test.txt"), osPath)
	})

	t.Run("handles nested paths", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		osPath, err := fsys.osPath("a/b/c/test.txt")
		be.NilErr(t, err)
		expected := filepath.Join(tmpDir, "a", "b", "c", "test.txt")
		be.Equal(t, expected, osPath)
	})

	t.Run("rejects invalid paths", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		invalidPaths := []string{
			"../outside",
			"/absolute",
		}

		for _, path := range invalidPaths {
			_, err := fsys.osPath(path)
			be.True(t, err != nil)
			be.Equal(t, fs.ErrInvalid, err)
		}
	})
}

func TestFS_SameBackend(t *testing.T) {
	t.Run("same root path returns true", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys1, err := NewFS(tmpDir)
		be.NilErr(t, err)
		// Trailing-slash and "."-suffixed variants of the same root.
		fsys2, err := NewFS(tmpDir + string(filepath.Separator))
		be.NilErr(t, err)
		fsys3, err := NewFS(filepath.Join(tmpDir, "."))
		be.NilErr(t, err)
		be.True(t, fsys1.SameBackend(fsys2))
		be.True(t, fsys1.SameBackend(fsys3))
		// Symmetric: the receiver can be either value.
		be.True(t, fsys2.SameBackend(fsys1))
		be.True(t, fsys3.SameBackend(fsys1))
	})

	t.Run("cleans trailing separators on raw paths", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys1 := &FS{path: tmpDir}
		fsys2 := &FS{path: tmpDir + string(filepath.Separator)}
		fsys3 := &FS{path: tmpDir + string(filepath.Separator) + "."}
		be.True(t, fsys1.SameBackend(fsys2))
		be.True(t, fsys1.SameBackend(fsys3))
	})

	t.Run("different root paths return false", func(t *testing.T) {
		fsys1, err := NewFS(t.TempDir())
		be.NilErr(t, err)
		fsys2, err := NewFS(t.TempDir())
		be.NilErr(t, err)
		be.False(t, fsys1.SameBackend(fsys2))
		be.False(t, fsys2.SameBackend(fsys1))
	})

	t.Run("non-local FS returns false", func(t *testing.T) {
		fsys, err := NewFS(t.TempDir())
		be.NilErr(t, err)
		other := ocflfs.NewWrapFS(os.DirFS(t.TempDir()))
		be.False(t, fsys.SameBackend(other))
	})
}

func TestFS_Implements_Interfaces(t *testing.T) {
	tmpDir := t.TempDir()
	fsys, err := NewFS(tmpDir)
	be.NilErr(t, err)

	// Test that Read operations work via DirEntriesFS
	ctx := context.Background()
	_, err = fsys.Write(ctx, "test.txt", strings.NewReader("test"))
	be.NilErr(t, err)

	// Should be able to open and read via the FS interface
	f, err := fsys.OpenFile(ctx, "test.txt")
	be.NilErr(t, err)
	defer f.Close()

	data, err := io.ReadAll(f)
	be.NilErr(t, err)
	be.Equal(t, "test", string(data))
}
