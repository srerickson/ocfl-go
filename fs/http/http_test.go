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
	"strconv"
	"testing"
	"time"

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

func TestContentToken(t *testing.T) {
	ctx := context.Background()
	lastMod := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	lm := "lm:" + strconv.FormatInt(lastMod.Unix(), 10)
	for name, tc := range map[string]struct {
		etag    string
		lastMod bool
		want    string
	}{
		"strong etag":               {etag: `"abc"`, lastMod: true, want: "etag:abc"},
		"weak etag":                 {etag: `W/"abc"`, lastMod: true, want: lm},
		"last-modified only":        {lastMod: true, want: lm},
		"neither":                   {want: ""},
		"weak etag only":            {etag: `W/"abc"`, want: ""},
		"strong etag only":          {etag: `"abc"`, want: "etag:abc"},
		"empty etag, last-modified": {etag: `""`, lastMod: true, want: lm},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.etag != "" {
					w.Header().Set("ETag", tc.etag)
				}
				if tc.lastMod {
					w.Header().Set("Last-Modified", lastMod.Format(http.TimeFormat))
				}
				w.Write([]byte("content"))
			}))
			defer srv.Close()
			info, err := ocflfs.StatFile(ctx, ocflhttp.New(srv.URL), "file.txt")
			be.NilErr(t, err)
			be.Equal(t, tc.want, ocflfs.ContentToken(info))
		})
	}
}

func TestMarshalText(t *testing.T) {
	t.Run("https url", func(t *testing.T) {
		text, err := ocflhttp.New("https://example.org/ocfl").MarshalText()
		be.NilErr(t, err)
		be.Equal(t, "https://example.org/ocfl", string(text))
	})
	t.Run("base url without a scheme", func(t *testing.T) {
		_, err := ocflhttp.New("example.org/ocfl").MarshalText()
		be.True(t, err != nil)
	})
}
