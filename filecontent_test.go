package ocfl_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/config"
	"github.com/srerickson/ocfl-go/fs/local"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

func TestFileContentMap(t *testing.T) {
	ctx := context.Background()
	digA := strings.Repeat("a", 128)
	digB := strings.Repeat("b", 128)
	t.Run("zero value", func(t *testing.T) {
		var c ocfl.FileContentMap
		srcFS, srcPath := c.GetContent(digA)
		be.Zero(t, srcFS)
		be.Zero(t, srcPath)
		be.Equal(t, 0, len(c.Digests()))
	})
	t.Run("files", func(t *testing.T) {
		fsys := ocflfs.DirFS(`testdata`)
		var c ocfl.FileContentMap
		c.StageFile(strings.ToUpper(digA), fsys, "content-fixture/hello.csv")
		c.StageFile(digB, fsys, "content-fixture/folder1/file.txt")
		be.DeepEqual(t, []string{digA, digB}, c.Digests())
		srcFS, srcPath := c.GetContent(digA)
		be.Equal[ocflfs.FS](t, fsys, srcFS)
		be.Equal(t, "content-fixture/hello.csv", srcPath)
		// replace a file
		c.StageFile(digB, fsys, "content-fixture/folder1/folder2/file2.txt")
		_, srcPath = c.GetContent(digB)
		be.Equal(t, "content-fixture/folder1/folder2/file2.txt", srcPath)
		c.Remove(digA)
		be.DeepEqual(t, []string{digB}, c.Digests())
		srcFS, _ = c.GetContent(digA)
		be.Zero(t, srcFS)
		c.DeleteFunc(func(dig string) bool { return dig == digB })
		be.Equal(t, 0, len(c.Digests()))
	})
	t.Run("FS values that can't be compared", func(t *testing.T) {
		// the struct type is comparable, but its value isn't
		fsys := mapFS{files: fstest.MapFS{"a.txt": &fstest.MapFile{Data: []byte("a")}}}
		var c ocfl.FileContentMap
		c.StageFile(digA, fsys, "a.txt")
		c.StageFile(digB, fsys, "a.txt")
		srcFS, srcPath := c.GetContent(digB)
		got, err := ocflfs.ReadAll(ctx, srcFS, srcPath)
		be.NilErr(t, err)
		be.Equal(t, "a", string(got))
	})
}

// mapFS is an ocflfs.FS with a field that holds a map.
type mapFS struct{ files fs.FS }

func (m mapFS) OpenFile(_ context.Context, name string) (fs.File, error) {
	return m.files.Open(name)
}

func TestFileContentMap_JSON(t *testing.T) {
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

	var c ocfl.FileContentMap
	c.StageFile(digC, unused, "c.txt")
	c.StageFile(digA, fsA, "dir/a.txt")
	c.StageFile(digB, fsB, "b.txt")
	c.StageFile(digC, fsA2, "c.txt")
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
	be.DeepEqual(t, []any{float64(1), "b.txt"}, savedJSON.Content[digB])
	be.DeepEqual(t, []any{float64(0), "c.txt"}, savedJSON.Content[digC])

	t.Run("load and save without opening", func(t *testing.T) {
		var loaded ocfl.FileContentMap
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
			Name    string               `json:"name"`
			Content *ocfl.FileContentMap `json:"content"`
		}
		data, err := json.Marshal(file{Name: "test", Content: &c})
		be.NilErr(t, err)
		var loaded file
		be.NilErr(t, json.Unmarshal(data, &loaded))
		be.Equal(t, "test", loaded.Name)
		be.DeepEqual(t, c.Digests(), loaded.Content.Digests())
	})
	t.Run("add after loading", func(t *testing.T) {
		var loaded ocfl.FileContentMap
		be.NilErr(t, json.Unmarshal(saved, &loaded))
		digD := strings.Repeat("d", 128)
		digE := strings.Repeat("e", 128)
		dirC := t.TempDir()
		fsC, err := local.NewFS(dirC)
		be.NilErr(t, err)
		loaded.StageFile(digD, fsC, "d.txt")
		// a directory that was loaded is saved once
		loaded.StageFile(digE, fsA2, "e.txt")
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
	t.Run("FS without MarshalText can't be saved", func(t *testing.T) {
		var c ocfl.FileContentMap
		c.StageFile(digA, ocflfs.DirFS(dirA), "a.txt")
		_, err := json.Marshal(c)
		be.Nonzero(t, err)
	})
	t.Run("invalid saved values", func(t *testing.T) {
		entry := func(e string) string {
			return `{"sources": ["file:///a"], "content": {"` + digA + `": ` + e + `}}`
		}
		for name, data := range map[string]string{
			"bad index":     entry(`[1, "a.txt"]`),
			"bad path":      entry(`[0, "../a.txt"]`),
			"bad source":    entry(`["0", "a.txt"]`),
			"one element":   entry(`[0]`),
			"four elements": entry(`[0, "a.txt", 1, "etag:a"]`),
			"empty source":  `{"sources": [""], "content": {}}`,
			"unknown field": `{"sources": [], "content": {}, "extra": 1}`,
		} {
			t.Run(name, func(t *testing.T) {
				var c ocfl.FileContentMap
				be.Nonzero(t, json.Unmarshal([]byte(data), &c))
			})
		}
	})
}

func TestFileContentMap_Open(t *testing.T) {
	ctx := context.Background()
	reg := config.Registry()
	digA := strings.Repeat("a", 128)
	digB := strings.Repeat("b", 128)
	// load returns a ContentMap loaded from JSON with content in two local
	// directories, and the directories.
	load := func(t *testing.T) (*ocfl.FileContentMap, string, string) {
		t.Helper()
		dirA, dirB := t.TempDir(), t.TempDir()
		data := `{"sources": [` + string(mustMarshal(t, localText(t, dirA))) + `, ` + string(mustMarshal(t, localText(t, dirB))) + `],
			"content": {"` + digA + `": [0, "a.txt"], "` + digB + `": [1, "b.txt"]}}`
		var c ocfl.FileContentMap
		be.NilErr(t, json.Unmarshal([]byte(data), &c))
		return &c, dirA, dirB
	}
	t.Run("open", func(t *testing.T) {
		c, dirA, dirB := load(t)
		be.NilErr(t, c.OpenFS(ctx, reg))
		srcFS, srcPath := c.GetContent(digA)
		be.Equal(t, dirA, srcFS.(*local.FS).Root())
		be.Equal(t, "a.txt", srcPath)
		srcFS, srcPath = c.GetContent(digB)
		be.Equal(t, dirB, srcFS.(*local.FS).Root())
		be.Equal(t, "b.txt", srcPath)
		// opening again doesn't open sources again
		be.NilErr(t, c.OpenFS(ctx, ocflfs.Registry{}))
		gotFS, _ := c.GetContent(digA)
		be.Equal(t, dirA, gotFS.(*local.FS).Root())
	})
	t.Run("missing directory", func(t *testing.T) {
		c, dirA, dirB := load(t)
		textA := localText(t, dirA)
		be.NilErr(t, os.Remove(dirA))
		err := c.OpenFS(ctx, reg)
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
		be.NilErr(t, c.OpenFS(ctx, reg))
	})
	t.Run("unknown scheme", func(t *testing.T) {
		c, _, _ := load(t)
		err := c.OpenFS(ctx, ocflfs.Registry{})
		be.True(t, errors.Is(err, ocflfs.ErrUnknownScheme))
	})
	t.Run("passwords aren't in errors", func(t *testing.T) {
		data := `{"sources": ["https://user:secret@example.org/ocfl"], "content": {"` + digA + `": [0, "a.txt"]}}`
		var c ocfl.FileContentMap
		be.NilErr(t, json.Unmarshal([]byte(data), &c))
		err := c.OpenFS(ctx, ocflfs.Registry{})
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

// fileStage is an update with content in files, which can be saved as JSON.
type fileStage struct {
	Update  *ocfl.ObjectUpdate   `json:"update"`
	Content *ocfl.FileContentMap `json:"content"`
}

func TestFileContentMap_JSON_withUpdate(t *testing.T) {
	ctx := context.Background()
	reg := config.Registry()
	contentFS, err := local.NewFS(filepath.Join(`testdata`, `content-fixture`))
	be.NilErr(t, err)
	objFS := testutil.TmpLocalFS(t)
	update, err := ocfl.NewUpdate(ctx, objFS, "obj", "obj")
	be.NilErr(t, err)
	stage := &fileStage{Update: update, Content: &ocfl.FileContentMap{}}
	be.NilErr(t, ocfl.StageFS(ctx, stage.Update, stage.Content, contentFS, "folder1", "files", ocfl.StageWithFixity(digest.MD5)))
	be.NilErr(t, ocfl.StageFile(ctx, stage.Update, stage.Content, contentFS, "hello.csv", "hello.csv"))

	saved, err := json.Marshal(stage)
	be.NilErr(t, err)
	var loaded fileStage
	be.NilErr(t, json.Unmarshal(saved, &loaded))
	be.True(t, stage.Update.NewState().Eq(loaded.Update.NewState()))
	be.DeepEqual(t, stage.Content.Digests(), loaded.Content.Digests())
	resaved, err := json.Marshal(loaded)
	be.NilErr(t, err)
	be.Equal(t, string(saved), string(resaved))

	// the loaded stage can be used to create the object once its content is
	// opened
	be.NilErr(t, loaded.Update.Finalize("v1", ocfl.User{Name: "Tester"}))
	be.NilErr(t, loaded.Content.OpenFS(ctx, reg))
	obj, err := loaded.Update.Apply(ctx, objFS, "obj", loaded.Content)
	be.NilErr(t, err)
	be.NilErr(t, ocfl.ValidateObject(ctx, objFS, obj.Path()).Err())
	be.DeepEqual(t, []string{"md5"}, obj.FixityAlgorithms())

	t.Run("invalid saved values", func(t *testing.T) {
		for name, data := range map[string]string{
			"invalid content": `{"update":` + string(mustMarshal(t, stage.Update)) + `,"content":{"extra":1}}`,
			"invalid update":  `{"update":{},"content":{"sources":[],"content":{}}}`,
		} {
			t.Run(name, func(t *testing.T) {
				var stage fileStage
				be.Nonzero(t, json.Unmarshal([]byte(data), &stage))
			})
		}
	})
}

func TestFileContentMap_JSON_missingContent(t *testing.T) {
	// a stage whose content directory has been removed can be loaded, changed,
	// saved and reverted: only opening its content fails.
	ctx := context.Background()
	contentFS := testutil.TmpLocalFS(t, filepath.Join(`testdata`, `content-fixture`))
	objFS := testutil.TmpLocalFS(t)
	upd, err := ocfl.NewUpdate(ctx, objFS, "obj", "obj")
	be.NilErr(t, err)
	stage := &fileStage{Update: upd, Content: &ocfl.FileContentMap{}}
	be.NilErr(t, ocfl.StageFS(ctx, upd, stage.Content, contentFS, "content-fixture", "."))
	be.NilErr(t, upd.Finalize("v1", ocfl.User{Name: "Tester"}))
	saved, err := json.Marshal(stage)
	be.NilErr(t, err)
	contentText, err := contentFS.MarshalText()
	be.NilErr(t, err)
	be.NilErr(t, contentFS.Close())
	be.NilErr(t, os.RemoveAll(contentFS.Root()))

	var loaded fileStage
	be.NilErr(t, json.Unmarshal(saved, &loaded))
	be.True(t, upd.NewState().Eq(loaded.Update.NewState()))
	be.True(t, loaded.Update.Finalized())
	resaved, err := json.Marshal(loaded)
	be.NilErr(t, err)
	be.Equal(t, string(saved), string(resaved))

	// content that was never opened is missing, so nothing is written
	_, err = loaded.Update.Apply(ctx, objFS, "obj", loaded.Content)
	be.True(t, errors.Is(err, ocfl.ErrMissingContent))
	_, err = ocflfs.ReadDir(ctx, objFS, "obj")
	be.True(t, errors.Is(err, fs.ErrNotExist))

	err = loaded.Content.OpenFS(ctx, config.Registry())
	be.True(t, errors.Is(err, fs.ErrNotExist))
	be.In(t, string(contentText), err.Error())

	be.NilErr(t, loaded.Update.Revert(ctx, objFS, "obj"))
	be.False(t, loaded.Update.Finalized())
	be.NilErr(t, loaded.Update.Remove("hello.csv"))
	_, err = json.Marshal(loaded)
	be.NilErr(t, err)
}
