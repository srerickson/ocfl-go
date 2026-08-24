package local_test

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
)

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
