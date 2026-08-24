package local

// Internal tests for localfs.go: osPath, the fs-path to OS-path translation
// every operation runs first. It is unexported, so the external local_test
// package cannot reach it.

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

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
