package local_test

// Tests for localfs.go: the FS type itself — construction, root reporting,
// backend identity, the interfaces it claims, and the directory listing it
// inherits from the wrapped io/fs.FS. Path translation (osPath) is unexported
// and tested in localfs_internal_test.go, as is SameBackend, which
// constructs an FS with a raw unexported root path.

import (
	"context"
	"errors"
	"io"
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

func TestNewFS(t *testing.T) {
	t.Run("creates FS with absolute path", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)
		be.Nonzero(t, fsys)

		root := fsys.Root()
		be.True(t, filepath.IsAbs(root))
	})

	t.Run("converts relative path to absolute", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Chdir(tmpDir)
		fsys, err := local.NewFS(".")
		be.NilErr(t, err)
		root := fsys.Root()
		be.True(t, filepath.IsAbs(root))
		be.True(t, strings.HasSuffix(root, filepath.Base(tmpDir)))
	})
}

func TestFS_Root(t *testing.T) {
	tmpDir := t.TempDir()
	fsys, err := local.NewFS(tmpDir)
	be.NilErr(t, err)

	root := fsys.Root()
	absPath, err := filepath.Abs(tmpDir)
	be.NilErr(t, err)
	be.Equal(t, absPath, root)
}

func TestFS_Implements_Interfaces(t *testing.T) {
	tmpDir := t.TempDir()
	fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(t.TempDir())
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
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)
		be.NilErr(t, os.Mkdir(filepath.Join(tmpDir, "empty"), 0o755))

		entries, err := ocflfs.ReadDir(context.Background(), fsys, "empty")
		be.NilErr(t, err)
		be.Equal(t, 0, len(entries))
		be.False(t, errors.Is(err, fs.ErrNotExist))
	})

	t.Run("iterator yields nothing for empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		fsys, err := local.NewFS(tmpDir)
		be.NilErr(t, err)
		be.NilErr(t, os.Mkdir(filepath.Join(tmpDir, "empty"), 0o755))

		// The DirEntries iterator itself must not yield a synthetic error
		// pair for an empty directory: zero pairs, no error.
		names, iterErr := collectDirEntries(t, fsys, "empty")
		be.NilErr(t, iterErr)
		be.Equal(t, 0, len(names))
	})

	t.Run("missing directory returns ErrNotExist", func(t *testing.T) {
		fsys, err := local.NewFS(t.TempDir())
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
		fsys, err := local.NewFS(tmpDir)
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
		fsys, err := local.NewFS(t.TempDir())
		be.NilErr(t, err)

		entries, err := ocflfs.ReadDir(context.Background(), fsys, "../escape")
		be.Equal(t, 0, len(entries))
		be.True(t, errors.Is(err, fs.ErrInvalid))
	})
}

// TestDirEntriesContract_Local runs the shared DirEntriesFS contract against
// the local backend.
func TestDirEntriesContract_Local(t *testing.T) {
	fsys := testutil.TmpLocalFS(t)
	testutil.TestDirEntriesContract(t, fsys)
}

// TestWalkFilesContract_Local runs the shared WalkFiles contract tests
// (internal/testutil) against the local backend. The local FS does not
// implement the FileWalker optimization (it has no WalkFiles method), so the
// package-level ocflfs.WalkFiles walks it through the DirEntries-based
// fileWalk path — the same generic walk every non-optimized backend gets.
//
// The error fixture makes the walk of "blocked" fail by planting a regular
// file where the walk expects to descend into a directory: the directory
// read then errors, which fileWalk must deliver as the pair's error element
// and stop on.
func TestWalkFilesContract_Local(t *testing.T) {
	fsys := testutil.TmpLocalFS(t)
	testutil.TestWalkFilesContract(t, fsys, testutil.WalkFilesContract{
		ErrWalk: func(t *testing.T) ocflfs.WriteFS {
			errFS := testutil.TmpLocalFS(t)
			if _, err := errFS.Write(context.Background(), "blocked", strings.NewReader("x")); err != nil {
				t.Fatalf("seeding blocked file: %v", err)
			}
			return errFS
		},
	})
}
