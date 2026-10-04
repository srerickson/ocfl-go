package ocfl_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
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

	t.Run("load and save without opening", func(t *testing.T) {
		var loaded ocfl.ContentMap
		be.NilErr(t, json.Unmarshal(saved, &loaded))
		be.DeepEqual(t, c.Digests(), loaded.Digests())
		// content isn't available until the sources are opened
		for _, dig := range loaded.Digests() {
			srcFS, srcPath := loaded.GetContent(dig)
			be.Zero(t, srcFS)
			be.Zero(t, srcPath)
		}
		resaved, err := json.Marshal(loaded)
		be.NilErr(t, err)
		be.Equal(t, string(saved), string(resaved))
	})
	t.Run("in a struct", func(t *testing.T) {
		type file struct {
			Name    string           `json:"name"`
			Content *ocfl.ContentMap `json:"content"`
		}
		data, err := json.Marshal(file{Name: "test", Content: &c})
		be.NilErr(t, err)
		var loaded file
		be.NilErr(t, json.Unmarshal(data, &loaded))
		be.Equal(t, "test", loaded.Name)
		be.DeepEqual(t, c.Digests(), loaded.Content.Digests())
	})
	t.Run("add after loading", func(t *testing.T) {
		var loaded ocfl.ContentMap
		be.NilErr(t, json.Unmarshal(saved, &loaded))
		digD := strings.Repeat("d", 128)
		digE := strings.Repeat("e", 128)
		dirC := t.TempDir()
		fsC, err := local.NewFS(dirC)
		be.NilErr(t, err)
		loaded.AddFile(digD, fsC, "d.txt")
		// a directory that was loaded is saved once
		loaded.AddFile(digE, fsA2, "e.txt")
		resaved, err := json.Marshal(loaded)
		be.NilErr(t, err)
		var resavedJSON struct {
			Sources []string
			Content map[string][]any
		}
		be.NilErr(t, json.Unmarshal(resaved, &resavedJSON))
		be.DeepEqual(t, append(savedJSON.Sources, localText(t, dirC)), resavedJSON.Sources)
		be.DeepEqual(t, []any{float64(2), "d.txt"}, resavedJSON.Content[digD])
		be.DeepEqual(t, []any{float64(0), "e.txt"}, resavedJSON.Content[digE])
	})
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
		for name, data := range map[string]string{
			"bad index":     `{"sources": ["file:///a"], "content": {"` + digA + `": [1, "a.txt"]}}`,
			"bad path":      `{"sources": ["file:///a"], "content": {"` + digA + `": [0, "../a.txt"]}}`,
			"missing index": `{"sources": ["file:///a"], "content": {"` + digA + `": ["a.txt"]}}`,
			"empty source":  `{"sources": [""], "content": {}}`,
			"unknown field": `{"sources": [], "content": {}, "extra": 1}`,
		} {
			t.Run(name, func(t *testing.T) {
				var c ocfl.ContentMap
				be.Nonzero(t, json.Unmarshal([]byte(data), &c))
			})
		}
	})
}

func TestContentMap_Open(t *testing.T) {
	ctx := context.Background()
	reg := config.Registry()
	digA := strings.Repeat("a", 128)
	digB := strings.Repeat("b", 128)
	// load returns a ContentMap loaded from JSON with content in two local
	// directories, and the directories.
	load := func(t *testing.T) (*ocfl.ContentMap, string, string) {
		t.Helper()
		dirA, dirB := t.TempDir(), t.TempDir()
		data := `{"sources": [` + string(mustMarshal(t, localText(t, dirA))) + `, ` + string(mustMarshal(t, localText(t, dirB))) + `],
			"content": {"` + digA + `": [0, "a.txt"], "` + digB + `": [1, "b.txt"]}}`
		var c ocfl.ContentMap
		be.NilErr(t, json.Unmarshal([]byte(data), &c))
		return &c, dirA, dirB
	}
	t.Run("open", func(t *testing.T) {
		c, dirA, dirB := load(t)
		be.NilErr(t, c.Open(ctx, reg))
		srcFS, srcPath := c.GetContent(digA)
		be.Equal(t, dirA, srcFS.(*local.FS).Root())
		be.Equal(t, "a.txt", srcPath)
		srcFS, srcPath = c.GetContent(digB)
		be.Equal(t, dirB, srcFS.(*local.FS).Root())
		be.Equal(t, "b.txt", srcPath)
		// opening again doesn't open sources again
		be.NilErr(t, c.Open(ctx, ocflfs.Registry{}))
		gotFS, _ := c.GetContent(digA)
		be.Equal(t, dirA, gotFS.(*local.FS).Root())
	})
	t.Run("missing directory", func(t *testing.T) {
		c, dirA, dirB := load(t)
		textA := localText(t, dirA)
		be.NilErr(t, os.Remove(dirA))
		err := c.Open(ctx, reg)
		be.True(t, errors.Is(err, fs.ErrNotExist))
		be.In(t, textA, err.Error())
		// the other source was opened
		srcFS, _ := c.GetContent(digB)
		be.Equal(t, dirB, srcFS.(*local.FS).Root())
		srcFS, _ = c.GetContent(digA)
		be.Zero(t, srcFS)
	})
	t.Run("removed content isn't opened", func(t *testing.T) {
		c, dirA, _ := load(t)
		be.NilErr(t, os.Remove(dirA))
		c.Remove(digA)
		be.NilErr(t, c.Open(ctx, reg))
	})
	t.Run("unknown scheme", func(t *testing.T) {
		c, _, _ := load(t)
		err := c.Open(ctx, ocflfs.Registry{})
		be.True(t, errors.Is(err, ocflfs.ErrUnknownScheme))
	})
	t.Run("passwords aren't in errors", func(t *testing.T) {
		data := `{"sources": ["https://user:secret@example.org/ocfl"], "content": {"` + digA + `": [0, "a.txt"]}}`
		var c ocfl.ContentMap
		be.NilErr(t, json.Unmarshal([]byte(data), &c))
		err := c.Open(ctx, ocflfs.Registry{})
		be.Nonzero(t, err)
		be.NotIn(t, "secret", err.Error())
	})
}

// localText returns the text that a local.FS for dir is saved as.
func localText(t *testing.T, dir string) string {
	t.Helper()
	text, err := local.MustNewFS(dir).MarshalText()
	be.NilErr(t, err)
	return string(text)
}
