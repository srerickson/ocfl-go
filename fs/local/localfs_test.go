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

		// An expired deadline surfaces as context.DeadlineExceeded itself,
		// not as some read error it happened to cause downstream.
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

		// Test various invalid paths
		invalidPaths := []string{
			"../outside",
			"/absolute",
			"./file",
		}

		for _, path := range invalidPaths {
			_, err = fsys.Write(ctx, path, strings.NewReader("content"))
			be.True(t, err != nil)

			var pathErr *fs.PathError
			be.True(t, errors.As(err, &pathErr))
		}
	})

	t.Run("sets permissions per umask for new files", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("content"))
		be.NilErr(t, err)

		// The contract for a new file is 0666 subject to the process umask,
		// matching os.Create. The reference file measures the umask instead
		// of assuming one: a hardcoded 0644 would only be right under 022.
		ref := filepath.Join(tmpDir, "ref.txt")
		be.NilErr(t, os.WriteFile(ref, []byte("x"), 0666))
		refInfo, err := os.Stat(ref)
		be.NilErr(t, err)

		info, err := os.Stat(filepath.Join(tmpDir, "test.txt"))
		be.NilErr(t, err)
		be.Equal(t, refInfo.Mode().Perm(), info.Mode().Perm())
		be.True(t, info.Mode().Perm()&0600 == 0600)
	})

	t.Run("preserves permissions of existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("first"))
		be.NilErr(t, err)
		be.NilErr(t, os.Chmod(filepath.Join(tmpDir, "test.txt"), 0600))

		// Overwriting keeps the target's mode rather than resetting it to
		// the umask-derived default: a private file stays private.
		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("second"))
		be.NilErr(t, err)

		data, err := os.ReadFile(filepath.Join(tmpDir, "test.txt"))
		be.NilErr(t, err)
		be.Equal(t, "second", string(data))
		info, err := os.Stat(filepath.Join(tmpDir, "test.txt"))
		be.NilErr(t, err)
		be.Equal(t, fs.FileMode(0600), info.Mode().Perm())
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

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "remove", pathErr.Op)
		be.Equal(t, ".", pathErr.Path)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = fsys.Remove(ctx, "test.txt")
		be.True(t, err != nil)

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
	})

	t.Run("rejects invalid path", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		err = fsys.Remove(ctx, "../outside")
		be.True(t, err != nil)

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
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

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, ".", pathErr.Path)
	})

	t.Run("succeeds on non-existent path", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		// RemoveAll should succeed even if path doesn't exist (like os.RemoveAll)
		err = fsys.RemoveAll(ctx, "nonexistent")
		be.NilErr(t, err)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = fsys.RemoveAll(ctx, "test")
		be.True(t, err != nil)

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
	})

	t.Run("rejects invalid path", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		err = fsys.RemoveAll(ctx, "../outside")
		be.True(t, err != nil)

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
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
