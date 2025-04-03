package http_test

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	ocflhttp "github.com/srerickson/ocfl-go/fs/http"
)

var (
	testdata = filepath.Join("..", "..", "testdata")

	//go:embed testdata/*
	testFS embed.FS
)

func TestHttpFS(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.FileServer(http.Dir(testdata)))
	fsys := ocflhttp.New(srv.URL)
	t.Run("read existing object", func(t *testing.T) {
		objPath := path.Join("object-fixtures", "1.1", "good-objects", "spec-ex-full")
		obj, err := ocfl.NewObject(ctx, fsys, objPath, ocfl.ObjectMustExist())
		be.NilErr(t, err)
		be.Equal(t, "ark:/12345/bcd987", obj.ID())
	})
	t.Run("stat file", func(t *testing.T) {
		info, err := ocflfs.StatFile(ctx, fsys, path.Join("content-fixture", "hello.csv"))
		be.NilErr(t, err)
		be.Equal(t, "hello.csv", info.Name())
		be.False(t, info.ModTime().IsZero())
		be.Equal(t, 15, info.Size())
		be.False(t, info.IsDir())
	})
	t.Run("invalid path", func(t *testing.T) {
		_, err := ocflfs.StatFile(ctx, fsys, path.Join("..", "hello.csv"))
		be.True(t, errors.Is(err, fs.ErrInvalid))

	})
	t.Run("not exist", func(t *testing.T) {
		_, err := ocflfs.StatFile(ctx, fsys, "missing")
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})

	t.Run("large file", func(t *testing.T) {
		name := path.Join("object-fixtures", "1.1", "good-objects", "updates_all_actions", "v1", "content", "my_content", "dracula.txt")
		info, err := ocflfs.StatFile(ctx, fsys, name)
		be.NilErr(t, err)
		buf, err := ocflfs.ReadAll(ctx, fsys, name)
		be.NilErr(t, err)
		be.Equal(t, info.Size(), int64(len(buf)))
	})
	srv.Close()
}

func TestEmbedFS(t *testing.T) {
	// Test the http.FS works for embed.FS backends.
	ctx := context.Background()
	srv := httptest.NewServer(http.FileServer(http.FS(testFS)))
	fsys := ocflhttp.New(srv.URL)
	f, err := fsys.OpenFile(ctx, "testdata/test.txt")
	be.NilErr(t, err)
	info, err := f.Stat()
	be.NilErr(t, err)
	be.Zero(t, info.ModTime())
	defer srv.Close()
}
