package fs_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

func TestContentToken(t *testing.T) {
	ctx := context.Background()
	t.Run("nil", func(t *testing.T) {
		be.Equal(t, "", ocflfs.ContentToken(nil))
	})
	t.Run("ContentTokener", func(t *testing.T) {
		info, err := os.Stat(t.TempDir())
		be.NilErr(t, err)
		be.Equal(t, "etag:abc", ocflfs.ContentToken(tokenInfo{FileInfo: info, token: "etag:abc"}))
		be.Equal(t, "", ocflfs.ContentToken(tokenInfo{FileInfo: info}))
	})
	t.Run("os stat", func(t *testing.T) {
		name := filepath.Join(t.TempDir(), "a.txt")
		be.NilErr(t, os.WriteFile(name, []byte("a"), 0o644))
		info, err := os.Stat(name)
		be.NilErr(t, err)
		be.True(t, strings.HasPrefix(ocflfs.ContentToken(info), "stat:"))
	})
	t.Run("no token", func(t *testing.T) {
		fsys := ocflfs.NewWrapFS(fstest.MapFS{"a.txt": {Data: []byte("a")}})
		info, err := ocflfs.StatFile(ctx, fsys, "a.txt")
		be.NilErr(t, err)
		be.Equal(t, "", ocflfs.ContentToken(info))
	})
	t.Run("DirFS", func(t *testing.T) {
		dir := t.TempDir()
		name := filepath.Join(dir, "a.txt")
		writeOld(t, name, "aaa")
		fsys := ocflfs.DirFS(dir)
		token := walkToken(t, fsys, "a.txt")
		be.True(t, strings.HasPrefix(token, "stat:"))
		info, err := ocflfs.StatFile(ctx, fsys, "a.txt")
		be.NilErr(t, err)
		be.Equal(t, token, ocflfs.ContentToken(info))
		// same size, different content
		be.NilErr(t, os.WriteFile(name, []byte("bbb"), 0o644))
		info, err = ocflfs.StatFile(ctx, fsys, "a.txt")
		be.NilErr(t, err)
		be.Unequal(t, token, ocflfs.ContentToken(info))
	})
}

// tokenInfo is an fs.FileInfo with a content token.
type tokenInfo struct {
	fs.FileInfo
	token string
}

func (i tokenInfo) ContentToken() string { return i.token }

// writeOld writes data to the file name and sets its modification time to an
// hour ago, so that rewriting it changes its modification time even on file
// systems with coarse timestamps.
func writeOld(t *testing.T, name, data string) {
	t.Helper()
	be.NilErr(t, os.WriteFile(name, []byte(data), 0o644))
	old := time.Now().Add(-time.Hour)
	be.NilErr(t, os.Chtimes(name, old, old))
}

// walkToken returns the content token for the file name in fsys from
// WalkFiles.
func walkToken(t *testing.T, fsys ocflfs.FS, name string) string {
	t.Helper()
	for ref, err := range ocflfs.WalkFiles(context.Background(), fsys, ".") {
		be.NilErr(t, err)
		if ref.FullPath() == name {
			return ocflfs.ContentToken(ref.Info)
		}
	}
	t.Fatalf("%q not found", name)
	return ""
}
