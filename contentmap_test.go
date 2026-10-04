package ocfl_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/config"
	"github.com/srerickson/ocfl-go/fs/local"
)

func TestContentMap(t *testing.T) {
	ctx := context.Background()
	digA := strings.Repeat("a", 128)
	digB := strings.Repeat("b", 128)
	t.Run("zero value", func(t *testing.T) {
		var c ocfl.ContentMap
		srcFS, srcPath := c.GetContent(digA)
		be.Zero(t, srcFS)
		be.Zero(t, srcPath)
		be.Equal(t, 0, len(c.Digests()))
	})
	t.Run("files and bytes", func(t *testing.T) {
		fsys := ocflfs.DirFS(`testdata`)
		var c ocfl.ContentMap
		c.AddFile(strings.ToUpper(digA), fsys, "content-fixture/hello.csv")
		c.AddBytes(digB, []byte("content"))
		be.DeepEqual(t, []string{digA, digB}, c.Digests())
		srcFS, srcPath := c.GetContent(digA)
		be.Equal[ocflfs.FS](t, fsys, srcFS)
		be.Equal(t, "content-fixture/hello.csv", srcPath)
		srcFS, srcPath = c.GetContent(digB)
		be.Nonzero(t, srcFS)
		got, err := ocflfs.ReadAll(ctx, srcFS, srcPath)
		be.NilErr(t, err)
		be.Equal(t, "content", string(got))
		// replace bytes with a file
		c.AddFile(digB, fsys, "content-fixture/folder1/file.txt")
		srcFS, srcPath = c.GetContent(digB)
		be.Equal[ocflfs.FS](t, fsys, srcFS)
		be.Equal(t, "content-fixture/folder1/file.txt", srcPath)
		c.Remove(digA)
		be.DeepEqual(t, []string{digB}, c.Digests())
		srcFS, _ = c.GetContent(digA)
		be.Zero(t, srcFS)
	})
}

func TestContentMap_JSON(t *testing.T) {
	ctx := context.Background()
	digA := strings.Repeat("a", 128)
	digB := strings.Repeat("b", 128)
	digC := strings.Repeat("c", 128)
	dirA, dirB := t.TempDir(), t.TempDir()
	fsA, err := local.NewFS(dirA)
	be.NilErr(t, err)
	fsB, err := local.NewFS(dirB)
	be.NilErr(t, err)
	fsA2, err := local.NewFS(dirA) // same directory as fsA
	be.NilErr(t, err)
	unused, err := local.NewFS(t.TempDir())
	be.NilErr(t, err)

	var c ocfl.ContentMap
	c.AddFile(digC, unused, "c.txt")
	c.AddFile(digA, fsA, "dir/a.txt")
	c.AddFile(digB, fsB, "b.txt")
	c.AddFile(digC, fsA2, "c.txt")
	saved, err := json.Marshal(c)
	be.NilErr(t, err)
	// unused sources aren't saved, and sources with the same text are saved
	// once.
	var savedJSON struct {
		Sources []string
		Content map[string][]any
	}
	be.NilErr(t, json.Unmarshal(saved, &savedJSON))
	be.Equal(t, 2, len(savedJSON.Sources))
	be.DeepEqual(t, []any{float64(0), "dir/a.txt"}, savedJSON.Content[digA])
	be.DeepEqual(t, []any{float64(0), "c.txt"}, savedJSON.Content[digC])

	loaded, err := ocfl.UnmarshalContentMap(ctx, saved, config.Opener())
	be.NilErr(t, err)
	be.DeepEqual(t, c.Digests(), loaded.Digests())
	for _, dig := range c.Digests() {
		wantFS, wantPath := c.GetContent(dig)
		gotFS, gotPath := loaded.GetContent(dig)
		be.Equal(t, wantPath, gotPath)
		be.Equal(t, wantFS.(*local.FS).Root(), gotFS.(*local.FS).Root())
	}

	t.Run("bytes can't be saved", func(t *testing.T) {
		var c ocfl.ContentMap
		c.AddBytes(digA, []byte("a"))
		_, err := json.Marshal(c)
		be.Nonzero(t, err)
	})
	t.Run("FS without MarshalText can't be saved", func(t *testing.T) {
		var c ocfl.ContentMap
		c.AddFile(digA, ocflfs.DirFS(dirA), "a.txt")
		_, err := json.Marshal(c)
		be.Nonzero(t, err)
	})
	t.Run("invalid saved values", func(t *testing.T) {
		source, err := json.Marshal("file://" + dirA)
		be.NilErr(t, err)
		for name, content := range map[string]string{
			"bad index":     `{"` + digA + `": [1, "a.txt"]}`,
			"bad path":      `{"` + digA + `": [0, "../a.txt"]}`,
			"missing index": `{"` + digA + `": ["a.txt"]}`,
		} {
			t.Run(name, func(t *testing.T) {
				data := `{"sources": [` + string(source) + `], "content": ` + content + `}`
				_, err := ocfl.UnmarshalContentMap(ctx, []byte(data), config.Opener())
				be.Nonzero(t, err)
			})
		}
		t.Run("source can't be opened", func(t *testing.T) {
			data := `{"sources": ["file:///does/not/exist"], "content": {}}`
			_, err := ocfl.UnmarshalContentMap(ctx, []byte(data), config.Opener())
			be.True(t, errors.Is(err, fs.ErrNotExist))
		})
	})
}
