package local_test

// Tests for write.go through the public API: FS.Write, including the shared
// cross-backend Write contract. Cases that assert on the atomic-write temp
// file or call tempFileName directly live in write_internal_test.go.

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
	"github.com/srerickson/ocfl-go/fs/local"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

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
		fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(tmpDir)
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

// TestWriteFSWriteContract_Local runs the shared WriteFS.Write contract
// against the local backend. The backend-specific assertions (atomicity as
// seen by a concurrent reader, temp-file placement, mode preservation) live
// in write_internal_test.go and write_unix_test.go.
func TestWriteFSWriteContract_Local(t *testing.T) {
	fsys := testutil.TmpLocalFS(t)
	testutil.TestWriteFSWriteContract(t, fsys, testutil.WriteFSWriteContract{
		WriteDotIsError: true,
	})
}
