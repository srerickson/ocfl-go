package local

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
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

	// NewFS now opens an os.Root on the directory, so a path that is not an
	// existing directory fails here rather than at the first operation on it.
	t.Run("rejects a missing directory", func(t *testing.T) {
		_, err := NewFS(filepath.Join(t.TempDir(), "no-such-dir"))
		be.True(t, err != nil)
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})

	t.Run("rejects a path that is not a directory", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "regular.txt")
		be.NilErr(t, os.WriteFile(file, []byte("x"), 0o644))
		_, err := NewFS(file)
		be.True(t, err != nil)
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

// TestFS_NameResolution replaces the old TestFS_osPath. Names are no longer
// turned into an OS path at all: every method hands the slash-separated name
// to the FS's os.Root, which resolves it one component at a time. What is
// left to pin is the observable half of what osPath did — a valid name lands
// under the storage root at the place its components name, and an invalid one
// is rejected as fs.ErrInvalid by every method rather than reaching the OS.
func TestFS_NameResolution(t *testing.T) {
	t.Run("nested name lands under the root", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		_, err = fsys.Write(ctx, "a/b/c/test.txt", strings.NewReader("nested"))
		be.NilErr(t, err)

		data, err := os.ReadFile(filepath.Join(tmpDir, "a", "b", "c", "test.txt"))
		be.NilErr(t, err)
		be.Equal(t, "nested", string(data))
	})

	t.Run("invalid names are rejected by every method", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		invalidNames := []string{
			"../outside",
			"/absolute",
			"./file",
			"",
		}
		for _, name := range invalidNames {
			ops := map[string]error{
				"write": func() error {
					_, err := fsys.Write(ctx, name, strings.NewReader("x"))
					return err
				}(),
				"remove":    fsys.Remove(ctx, name),
				"removeall": fsys.RemoveAll(ctx, name),
				"openfile": func() error {
					_, err := fsys.OpenFile(ctx, name)
					return err
				}(),
			}
			for _, entry := range slices.Sorted(maps.Keys(ops)) {
				opErr := ops[entry]
				if opErr == nil {
					t.Fatalf("%s(%q) = nil, want an error", entry, name)
				}
				var pathErr *fs.PathError
				be.True(t, errors.As(opErr, &pathErr))
				be.Equal(t, name, pathErr.Path)
				if !errors.Is(opErr, fs.ErrInvalid) {
					t.Fatalf("%s(%q) = %v, want fs.ErrInvalid", entry, name, opErr)
				}
			}
			// DirEntries reports the same rejection through its iterator.
			var dirErr error
			for _, err := range fsys.DirEntries(ctx, name) {
				if err != nil {
					dirErr = err
				}
			}
			if !errors.Is(dirErr, fs.ErrInvalid) {
				t.Fatalf("DirEntries(%q) = %v, want fs.ErrInvalid", name, dirErr)
			}
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

// TestFS_Write_ConcurrentNestedWrites pins the tolerance mkdirAll adds over
// Root.MkdirAll. Root.MkdirAll opens each intermediate component and, on
// ENOENT, calls mkdirat exactly once, reporting EEXIST as an error if another
// goroutine created that directory in between -- so two concurrent writes
// under a common new prefix fail with "file exists". Writing an object
// version does exactly that, which is how this surfaced.
func TestFS_Write_ConcurrentNestedWrites(t *testing.T) {
	fsys, err := NewFS(t.TempDir())
	be.NilErr(t, err)
	ctx := context.Background()

	// A deep shared prefix widens the window: every writer must create the
	// same chain of missing parents.
	const prefix = "obj/v2/content/docs/deep"
	const writers = 16

	start := make(chan struct{})
	errs := make(chan error, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			name := prefix + "/file-" + strconv.Itoa(i) + ".txt"
			_, err := fsys.Write(ctx, name, strings.NewReader(name))
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		be.NilErr(t, err)
	}

	for i := range writers {
		name := prefix + "/file-" + strconv.Itoa(i) + ".txt"
		data, err := os.ReadFile(filepath.Join(fsys.Root(), filepath.FromSlash(name)))
		be.NilErr(t, err)
		be.Equal(t, name, string(data))
	}
}

// TestFS_Write_ParentExistsAsFile pins the other side of mkdirAll's EEXIST
// handling: tolerating an existing directory must not tolerate an existing
// regular file standing where a parent directory belongs.
func TestFS_Write_ParentExistsAsFile(t *testing.T) {
	fsys, err := NewFS(t.TempDir())
	be.NilErr(t, err)
	ctx := context.Background()

	_, err = fsys.Write(ctx, "blocker", strings.NewReader("x"))
	be.NilErr(t, err)

	_, err = fsys.Write(ctx, "blocker/child.txt", strings.NewReader("y"))
	be.True(t, err != nil)
	var pathErr *fs.PathError
	be.True(t, errors.As(err, &pathErr))
	be.Equal(t, "write", pathErr.Op)
	be.Equal(t, "blocker/child.txt", pathErr.Path)
}

// TestFS_Close pins the descriptor lifecycle NewFS introduces: Close releases
// the root, after which operations fail rather than silently succeeding, and
// a second Close is harmless.
func TestFS_Close(t *testing.T) {
	fsys, err := NewFS(t.TempDir())
	be.NilErr(t, err)
	ctx := context.Background()

	_, err = fsys.Write(ctx, "before.txt", strings.NewReader("x"))
	be.NilErr(t, err)

	be.NilErr(t, fsys.Close())
	be.NilErr(t, fsys.Close()) // idempotent

	_, err = fsys.Write(ctx, "after.txt", strings.NewReader("x"))
	be.True(t, err != nil)
	_, err = fsys.OpenFile(ctx, "before.txt")
	be.True(t, err != nil)
}

// TestMustNewFS covers both halves of the Must contract.
func TestMustNewFS(t *testing.T) {
	t.Run("returns an FS for an existing directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys := MustNewFS(tmpDir)
		be.Equal(t, tmpDir, fsys.Root())
	})

	t.Run("panics when the directory is missing", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("MustNewFS did not panic on a missing directory")
			}
		}()
		MustNewFS(filepath.Join(t.TempDir(), "no-such-dir"))
	})
}
