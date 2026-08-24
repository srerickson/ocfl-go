package local

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// tmpFilesIn returns the temp files (".<base>.tmp-<random>") left in dir by
// an atomic write. After Write returns — successfully or not — this must
// always be empty: any match is a leaked temp file.
func tmpFilesIn(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".*.tmp-*"))
	be.NilErr(t, err)
	return matches
}

// cancelChunkReader yields two chunks of data and cancels the context after
// the second one. A Write that checks ctx between chunks aborts with
// context.Canceled instead of committing the file to the final path.
type cancelChunkReader struct {
	cancel context.CancelFunc
	chunks int
}

func (r *cancelChunkReader) Read(p []byte) (int, error) {
	r.chunks++
	if r.chunks > 1 {
		r.cancel()
	}
	if r.chunks >= 3 {
		return 0, io.EOF
	}
	return copy(p, "chunk-data"), nil
}

// failAfterReader yields data once, then fails with err on the next read.
type failAfterReader struct {
	data []byte
	err  error
	sent bool
}

func (r *failAfterReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, r.data), nil
	}
	return 0, r.err
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

	t.Run("sets permissions per umask for new files", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("content"))
		be.NilErr(t, err)

		// The contract for a new file is 0666 subject to the process
		// umask, matching plain os.Create/open(2) semantics. Compare
		// against a reference file created with os.WriteFile(0666) so the
		// assertion holds under any umask (a hardcoded 0644 would only be
		// correct under umask 022).
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

		// Overwriting an existing file must keep its permissions rather
		// than resetting them to the umask-derived default.
		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("second"))
		be.NilErr(t, err)

		data, err := os.ReadFile(filepath.Join(tmpDir, "test.txt"))
		be.NilErr(t, err)
		be.Equal(t, "second", string(data))
		info, err := os.Stat(filepath.Join(tmpDir, "test.txt"))
		be.NilErr(t, err)
		be.Equal(t, fs.FileMode(0600), info.Mode().Perm())
	})

	t.Run("aborts on mid-write cancellation with no partial file", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		// The reader cancels the context after the second chunk. A Write
		// that fails to check ctx between chunks would keep reading to EOF
		// and commit the file, so this pins the mid-write cancellation
		// check and the no-partial-file property for a new target.
		_, err = fsys.Write(ctx, "test.txt", &cancelChunkReader{cancel: cancel})
		be.True(t, errors.Is(err, context.Canceled))

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "write", pathErr.Op)
		be.Equal(t, "test.txt", pathErr.Path)

		if _, statErr := os.Stat(filepath.Join(tmpDir, "test.txt")); !os.IsNotExist(statErr) {
			t.Fatalf("partial file visible at final path: %v", statErr)
		}
		be.Equal(t, 0, len(tmpFilesIn(t, tmpDir)))
	})

	t.Run("aborts on mid-write cancellation keeping existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("old-content"))
		be.NilErr(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		_, err = fsys.Write(ctx, "test.txt", &cancelChunkReader{cancel: cancel})
		be.True(t, errors.Is(err, context.Canceled))

		// The previous complete file must be untouched: cancellation
		// leaves either the old file or the new complete file, never a
		// truncated one.
		data, err := os.ReadFile(filepath.Join(tmpDir, "test.txt"))
		be.NilErr(t, err)
		be.Equal(t, "old-content", string(data))
		be.Equal(t, 0, len(tmpFilesIn(t, tmpDir)))
	})

	t.Run("reader error leaves old file intact and cleans up temp", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		_, err = fsys.Write(ctx, "test.txt", strings.NewReader("old-content"))
		be.NilErr(t, err)

		// A source that fails after producing some bytes must not truncate
		// or partially overwrite the existing file (the pre-atomic Write
		// opened with O_TRUNC and would have left it truncated).
		errBoom := errors.New("read boom")
		_, err = fsys.Write(ctx, "test.txt", &failAfterReader{data: []byte("partial"), err: errBoom})
		be.True(t, errors.Is(err, errBoom))

		data, err := os.ReadFile(filepath.Join(tmpDir, "test.txt"))
		be.NilErr(t, err)
		be.Equal(t, "old-content", string(data))
		be.Equal(t, 0, len(tmpFilesIn(t, tmpDir)))
	})

	t.Run("reader error creates no final file", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		errBoom := errors.New("read boom")
		_, err = fsys.Write(ctx, "new.txt", &failAfterReader{data: []byte("partial"), err: errBoom})
		be.True(t, errors.Is(err, errBoom))

		// New target: failure must leave no file at the final path and no
		// temp file behind.
		if _, statErr := os.Stat(filepath.Join(tmpDir, "new.txt")); !os.IsNotExist(statErr) {
			t.Fatalf("partial file visible at final path: %v", statErr)
		}
		be.Equal(t, 0, len(tmpFilesIn(t, tmpDir)))
	})

	t.Run("concurrent writes never interleave or collide", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		ctx := context.Background()
		// Ten writers race on the same path. With atomic temp+rename each
		// writer installs a complete file, so the final content must equal
		// one of the inputs in full, and the O_EXCL temp creation must not
		// fail on collisions.
		const writers = 10
		inputs := make([]string, writers)
		for i := range writers {
			inputs[i] = strings.Repeat(string(rune('A'+i)), 64*1024+i)
		}
		var wg sync.WaitGroup
		errs := make([]error, writers)
		for i := range writers {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, errs[i] = fsys.Write(ctx, "shared.txt", strings.NewReader(inputs[i]))
			}(i)
		}
		wg.Wait()
		for _, err := range errs {
			be.NilErr(t, err)
		}

		data, err := os.ReadFile(filepath.Join(tmpDir, "shared.txt"))
		be.NilErr(t, err)
		got := string(data)
		matched := false
		for _, in := range inputs {
			if got == in {
				matched = true
				break
			}
		}
		be.True(t, matched)
		be.Equal(t, 0, len(tmpFilesIn(t, tmpDir)))
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

	t.Run("removes symlink without following it outside the root", func(t *testing.T) {
		// Regression pin for RemoveAll symlink safety. A symlink at a path
		// inside the storage root must be removed as a link and never
		// followed to its target — even when that target is a directory
		// outside the root. (The trailing slash in the implementation is
		// not what provides this: os.RemoveAll strips trailing separators
		// and unlinks the entry itself, so the slash changes nothing on any
		// supported Go. The property pinned here is the one the slash was
		// intended to protect: RemoveAll on this path must never delete
		// anything outside the storage root. Any rewrite that follows the
		// link — with or without the slash — fails this test by deleting
		// the external target.)
		root := t.TempDir()
		fsys, err := NewFS(root)
		be.NilErr(t, err)

		// External target: a sibling temporary directory outside the
		// storage root, containing a known file.
		ext := filepath.Join(t.TempDir(), "external")
		be.NilErr(t, os.MkdirAll(ext, 0o755))
		victim := filepath.Join(ext, "victim.txt")
		be.NilErr(t, os.WriteFile(victim, []byte("precious"), 0o644))

		// A symlink inside the storage root pointing at the external
		// directory. Production RemoveAll appends the trailing slash to the
		// OS path, so this exercises the production call path exactly.
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

// collectDirEntries drains the DirEntries iterator on fsys for name and returns
// the yielded entry names and the first non-nil error (if any).
func collectDirEntries(t *testing.T, fsys ocflfs.DirEntriesFS, name string) ([]string, error) {
	t.Helper()
	var names []string
	var iterErr error
	for entry, err := range fsys.DirEntries(context.Background(), name) {
		if entry != nil {
			names = append(names, entry.Name())
		}
		if err != nil {
			iterErr = err
		}
	}
	return names, iterErr
}

// TestFS_DirEntries pins the readdir semantics of the local backend: reading an
// existing-but-empty directory yields zero entries and no error (never
// fs.ErrNotExist), while reading a directory that does not exist yields an
// error that satisfies errors.Is(err, fs.ErrNotExist).
func TestFS_DirEntries(t *testing.T) {
	t.Run("empty top-level directory returns zero entries", func(t *testing.T) {
		fsys, err := NewFS(t.TempDir())
		be.NilErr(t, err)

		// The root directory exists and is empty: ReadDir must return an
		// empty result with no error — an empty dir is not a missing dir.
		entries, err := ocflfs.ReadDir(context.Background(), fsys, ".")
		be.NilErr(t, err)
		be.Equal(t, 0, len(entries))
		be.False(t, errors.Is(err, fs.ErrNotExist))
	})

	t.Run("empty nested directory returns zero entries", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)
		be.NilErr(t, os.Mkdir(filepath.Join(tmpDir, "empty"), 0o755))

		entries, err := ocflfs.ReadDir(context.Background(), fsys, "empty")
		be.NilErr(t, err)
		be.Equal(t, 0, len(entries))
		be.False(t, errors.Is(err, fs.ErrNotExist))
	})

	t.Run("iterator yields nothing for empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)
		be.NilErr(t, os.Mkdir(filepath.Join(tmpDir, "empty"), 0o755))

		// The DirEntries iterator itself must not yield a synthetic error
		// pair for an empty directory: zero pairs, no error.
		names, iterErr := collectDirEntries(t, fsys, "empty")
		be.NilErr(t, iterErr)
		be.Equal(t, 0, len(names))
	})

	t.Run("missing directory returns ErrNotExist", func(t *testing.T) {
		fsys, err := NewFS(t.TempDir())
		be.NilErr(t, err)

		entries, err := ocflfs.ReadDir(context.Background(), fsys, "missing")
		be.Equal(t, 0, len(entries))
		be.True(t, errors.Is(err, fs.ErrNotExist))

		// The iterator yields exactly one (nil, err) pair for a missing dir.
		names, iterErr := collectDirEntries(t, fsys, "missing")
		be.Equal(t, 0, len(names))
		be.True(t, errors.Is(iterErr, fs.ErrNotExist))
	})

	t.Run("directory with entries returns sorted names", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := NewFS(tmpDir)
		be.NilErr(t, err)

		for _, name := range []string{"c.txt", "a.txt", "b.txt"} {
			_, err = fsys.Write(context.Background(), name, strings.NewReader("x"))
			be.NilErr(t, err)
		}
		be.NilErr(t, os.Mkdir(filepath.Join(tmpDir, "sub"), 0o755))

		entries, err := ocflfs.ReadDir(context.Background(), fsys, ".")
		be.NilErr(t, err)
		names := make([]string, len(entries))
		for i, entry := range entries {
			names[i] = entry.Name()
		}
		be.AllEqual(t, []string{"a.txt", "b.txt", "c.txt", "sub"}, names)
	})

	t.Run("invalid path returns ErrInvalid", func(t *testing.T) {
		fsys, err := NewFS(t.TempDir())
		be.NilErr(t, err)

		entries, err := ocflfs.ReadDir(context.Background(), fsys, "../escape")
		be.Equal(t, 0, len(entries))
		be.True(t, errors.Is(err, fs.ErrInvalid))
	})
}
