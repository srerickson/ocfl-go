package ocfl_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
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

func TestStage_AddFS(t *testing.T) {
	ctx := context.Background()
	testdataFS := ocflfs.DirFS(`testdata`)
	newStage := func(t *testing.T) *ocfl.Stage {
		upd, err := ocfl.NewUpdate(ctx, ocflfs.DirFS(t.TempDir()), "obj", "obj", ocfl.UpdateWithDigestAlgorithm(digest.SHA256))
		be.NilErr(t, err)
		return ocfl.NewStage(upd)
	}
	t.Run("top-level directory", func(t *testing.T) {
		stage := newStage(t)
		be.NilErr(t, stage.AddFS(ctx, testdataFS, "content-fixture", ".", ocfl.StageWithFixity(digest.MD5)))
		// hidden files are ignored
		be.DeepEqual(t, []string{
			"folder1/file.txt",
			"folder1/folder2/file2.txt",
			"folder1/folder2/sculpture-stone-face-head-888027.jpg",
			"hello.csv",
		}, stage.Update.State().AllPaths())
		// every new digest has content and fixity
		state := stage.Update.State()
		be.Equal(t, 3, len(state))
		be.DeepEqual(t, sortedKeys(state), stage.Content.Digests())
		for dig := range state {
			be.Nonzero(t, stage.Update.Fixity(dig)["md5"])
			srcFS, srcPath := stage.Content.GetContent(dig)
			be.Nonzero(t, srcFS)
			_, err := ocflfs.StatFile(ctx, srcFS, srcPath)
			be.NilErr(t, err)
		}
	})
	t.Run("subdirectory", func(t *testing.T) {
		stage := newStage(t)
		be.NilErr(t, stage.AddFS(ctx, testdataFS, "content-fixture/folder1", "a/b"))
		be.DeepEqual(t, []string{
			"a/b/file.txt",
			"a/b/folder2/file2.txt",
			"a/b/folder2/sculpture-stone-face-head-888027.jpg",
		}, stage.Update.State().AllPaths())
	})
	t.Run("conflicting paths add nothing", func(t *testing.T) {
		stage := newStage(t)
		be.NilErr(t, stage.AddBytes("folder1", []byte("a file, not a directory")))
		err := stage.AddFS(ctx, testdataFS, "content-fixture", ".")
		be.Nonzero(t, err)
		be.DeepEqual(t, []string{"folder1"}, stage.Update.State().AllPaths())
		be.Equal(t, 1, len(stage.Content.Digests()))
	})
	t.Run("missing directory", func(t *testing.T) {
		stage := newStage(t)
		err := stage.AddFS(ctx, testdataFS, "missing", ".")
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})
	t.Run("hidden files", func(t *testing.T) {
		hiddenFS := ocflfs.NewWrapFS(fstest.MapFS{
			"a.txt":       &fstest.MapFile{Data: []byte("a")},
			".b.txt":      &fstest.MapFile{Data: []byte("b")},
			".dir/c.txt":  &fstest.MapFile{Data: []byte("c")},
			"dir/.d.txt":  &fstest.MapFile{Data: []byte("d")},
			"dir/e/f.txt": &fstest.MapFile{Data: []byte("f")},
		})
		stage := newStage(t)
		be.NilErr(t, stage.AddFS(ctx, hiddenFS, ".", "."))
		be.DeepEqual(t, []string{"a.txt", "dir/e/f.txt"}, stage.Update.State().AllPaths())
		stage = newStage(t)
		be.NilErr(t, stage.AddFS(ctx, hiddenFS, ".", ".", ocfl.StageWithHidden()))
		be.DeepEqual(t, []string{
			".b.txt",
			".dir/c.txt",
			"a.txt",
			"dir/.d.txt",
			"dir/e/f.txt",
		}, stage.Update.State().AllPaths())
		be.Equal(t, 5, len(stage.Content.Digests()))
	})
	t.Run("hidden symlink adds nothing", func(t *testing.T) {
		// content-fixture's hidden directory has a symlink, which can't be
		// added
		stage := newStage(t)
		err := stage.AddFS(ctx, testdataFS, "content-fixture", ".", ocfl.StageWithHidden())
		be.True(t, errors.Is(err, ocflfs.ErrFileType))
		be.Equal(t, 0, len(stage.Update.State().AllPaths()))
		be.Equal(t, 0, len(stage.Content.Digests()))
	})
	t.Run("filter", func(t *testing.T) {
		stage := newStage(t)
		noHiddenDir := func(ref *ocflfs.FileRef) bool { return path.Base(path.Dir(ref.Path)) != ".hidden_folder" }
		be.NilErr(t, stage.AddFS(ctx, testdataFS, "content-fixture/folder1", ".", ocfl.StageWithFilter(noHiddenDir)))
		// the filter replaces the default, so hidden files that pass it are
		// added
		be.DeepEqual(t, []string{
			"file.txt",
			"folder2/.hidden_file.txt",
			"folder2/file2.txt",
			"folder2/sculpture-stone-face-head-888027.jpg",
		}, stage.Update.State().AllPaths())
	})
	t.Run("go limit", func(t *testing.T) {
		serial := newStage(t)
		be.NilErr(t, serial.AddFS(ctx, testdataFS, "content-fixture", ".", ocfl.StageWithFixity(digest.MD5)))
		concurrent := newStage(t)
		be.NilErr(t, concurrent.AddFS(ctx, testdataFS, "content-fixture", ".",
			ocfl.StageWithFixity(digest.MD5), ocfl.StageWithGoLimit(4)))
		be.True(t, serial.Update.State().Eq(concurrent.Update.State()))
		be.DeepEqual(t, serial.Content.Digests(), concurrent.Content.Digests())
		for dig := range serial.Update.State() {
			be.DeepEqual(t, serial.Update.Fixity(dig), concurrent.Update.Fixity(dig))
		}
	})
	t.Run("digest error adds nothing", func(t *testing.T) {
		badFS := &openErrFS{WrapFS: ocflfs.DirFS(`testdata`), name: "content-fixture/hello.csv"}
		stage := newStage(t)
		err := stage.AddFS(ctx, badFS, "content-fixture", ".", ocfl.StageWithGoLimit(4))
		be.True(t, errors.Is(err, errOpen))
		be.Equal(t, 0, len(stage.Update.State().AllPaths()))
		be.Equal(t, 0, len(stage.Content.Digests()))
	})
}

var errOpen = errors.New("open failed")

// openErrFS returns errOpen when the file name is opened.
type openErrFS struct {
	*ocflfs.WrapFS
	name string
}

func (fsys *openErrFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if name == fsys.name {
		return nil, errOpen
	}
	return fsys.WrapFS.OpenFile(ctx, name)
}

func TestStage_AddFile(t *testing.T) {
	ctx := context.Background()
	testdataFS := ocflfs.DirFS(`testdata`)
	upd, err := ocfl.NewUpdate(ctx, ocflfs.DirFS(t.TempDir()), "obj", "obj")
	be.NilErr(t, err)
	stage := ocfl.NewStage(upd)
	be.NilErr(t, stage.AddFile(ctx, testdataFS, "content-fixture/hello.csv", "data/hello.csv", ocfl.StageWithFixity(digest.SIZE)))
	dig := upd.State().DigestFor("data/hello.csv")
	be.Nonzero(t, dig)
	be.Nonzero(t, upd.Fixity(dig)["size"])
	srcFS, srcPath := stage.Content.GetContent(dig)
	be.Equal[ocflfs.FS](t, testdataFS, srcFS)
	be.Equal(t, "content-fixture/hello.csv", srcPath)
	t.Run("missing file", func(t *testing.T) {
		err := stage.AddFile(ctx, testdataFS, "missing", "missing")
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})
	t.Run("hidden file", func(t *testing.T) {
		name := "content-fixture/folder1/folder2/.hidden_file.txt"
		be.NilErr(t, stage.AddFile(ctx, testdataFS, name, ".hidden.txt"))
		be.Nonzero(t, upd.State().DigestFor(".hidden.txt"))
	})
}

func TestStage_Edits(t *testing.T) {
	ctx := context.Background()
	fixture := filepath.Join(objectFixturesPath, `1.1`, `good-objects`, `spec-ex-full`)
	obj, err := ocfl.NewObject(ctx, ocflfs.DirFS(fixture), ".")
	be.NilErr(t, err)
	t.Run("content already in the object", func(t *testing.T) {
		stage := ocfl.NewStage(obj.NewUpdate())
		// empty2.txt's content is in the object: only the state changes
		be.NilErr(t, stage.AddBytes("empty3.txt", []byte{}, digest.MD5))
		be.Equal(t, stage.Update.State().DigestFor("empty2.txt"), stage.Update.State().DigestFor("empty3.txt"))
		be.Equal(t, 0, len(stage.Content.Digests()))
		be.Zero(t, stage.Update.Fixity(stage.Update.State().DigestFor("empty3.txt")))
	})
	t.Run("remove drops content and fixity", func(t *testing.T) {
		stage := ocfl.NewStage(obj.NewUpdate())
		be.NilErr(t, stage.AddBytes("new/a.txt", []byte("a"), digest.MD5))
		be.NilErr(t, stage.AddBytes("new/b.txt", []byte("a")))
		dig := stage.Update.State().DigestFor("new/a.txt")
		be.NilErr(t, stage.Remove("new/a.txt"))
		// content is still used by new/b.txt
		be.DeepEqual(t, []string{dig}, stage.Content.Digests())
		be.Nonzero(t, stage.Update.Fixity(dig))
		be.NilErr(t, stage.Remove("new"))
		be.Equal(t, 0, len(stage.Content.Digests()))
		be.Zero(t, stage.Update.Fixity(dig))
		err := stage.Remove("new")
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})
	t.Run("replacing a file drops its content", func(t *testing.T) {
		stage := ocfl.NewStage(obj.NewUpdate())
		be.NilErr(t, stage.AddBytes("a.txt", []byte("a")))
		be.NilErr(t, stage.AddBytes("a.txt", []byte("b")))
		be.DeepEqual(t, []string{stage.Update.State().DigestFor("a.txt")}, stage.Content.Digests())
	})
	t.Run("clear", func(t *testing.T) {
		stage := ocfl.NewStage(obj.NewUpdate())
		be.NilErr(t, stage.AddBytes("new/a.txt", []byte("a"), digest.MD5))
		be.NilErr(t, stage.AddFS(ctx, ocflfs.DirFS(`testdata`), "content-fixture", "fixture"))
		be.NilErr(t, stage.Clear())
		be.Equal(t, 0, len(stage.Update.State()))
		be.Equal(t, 0, len(stage.Content.Digests()))
		// the content can be added again
		be.NilErr(t, stage.AddBytes("new/a.txt", []byte("a")))
		be.Equal(t, 1, len(stage.Content.Digests()))
	})
	t.Run("rename keeps content", func(t *testing.T) {
		stage := ocfl.NewStage(obj.NewUpdate())
		be.NilErr(t, stage.AddBytes("new/a.txt", []byte("a")))
		be.NilErr(t, stage.Rename("new", "newer"))
		dig := stage.Update.State().DigestFor("newer/a.txt")
		be.Nonzero(t, dig)
		be.DeepEqual(t, []string{dig}, stage.Content.Digests())
	})
}

func TestStage_JSON(t *testing.T) {
	ctx := context.Background()
	reg := config.Registry()
	contentFS, err := local.NewFS(filepath.Join(`testdata`, `content-fixture`))
	be.NilErr(t, err)
	objFS := testutil.TmpLocalFS(t)
	upd, err := ocfl.NewUpdate(ctx, objFS, "obj", "obj")
	be.NilErr(t, err)
	stage := ocfl.NewStage(upd)
	be.NilErr(t, stage.AddFS(ctx, contentFS, "folder1", "files", ocfl.StageWithFixity(digest.MD5)))
	be.NilErr(t, stage.AddFile(ctx, contentFS, "hello.csv", "hello.csv"))

	saved, err := json.Marshal(stage)
	be.NilErr(t, err)
	var loaded ocfl.Stage
	be.NilErr(t, json.Unmarshal(saved, &loaded))
	be.True(t, stage.Update.State().Eq(loaded.Update.State()))
	be.DeepEqual(t, stage.Content.Digests(), loaded.Content.Digests())
	resaved, err := json.Marshal(loaded)
	be.NilErr(t, err)
	be.Equal(t, string(saved), string(resaved))

	// the loaded stage can be used to create the object once its content is
	// opened
	be.NilErr(t, loaded.Update.Finalize("v1", ocfl.User{Name: "Tester"}))
	be.NilErr(t, loaded.Content.Open(ctx, reg))
	obj, err := loaded.Update.Apply(ctx, objFS, "obj", loaded.Content)
	be.NilErr(t, err)
	be.NilErr(t, ocfl.ValidateObject(ctx, objFS, obj.Path()).Err())
	be.DeepEqual(t, []string{"md5"}, obj.FixityAlgorithms())

	t.Run("file sizes", func(t *testing.T) {
		var savedJSON struct {
			Content struct{ Content map[string][]any }
		}
		be.NilErr(t, json.Unmarshal(saved, &savedJSON))
		be.Equal(t, 3, len(savedJSON.Content.Content))
		for _, entry := range savedJSON.Content.Content {
			be.Equal(t, 3, len(entry))
			info, err := os.Stat(filepath.Join(contentFS.Root(), entry[1].(string)))
			be.NilErr(t, err)
			be.Equal(t, float64(info.Size()), entry[2].(float64))
		}
	})
	t.Run("saved without file sizes", func(t *testing.T) {
		// stages saved before sizes were recorded can be loaded and applied
		var old map[string]any
		be.NilErr(t, json.Unmarshal(saved, &old))
		content := old["content"].(map[string]any)["content"].(map[string]any)
		for dig, entry := range content {
			content[dig] = entry.([]any)[:2]
		}
		var loaded ocfl.Stage
		be.NilErr(t, json.Unmarshal(mustMarshal(t, old), &loaded))
		be.DeepEqual(t, stage.Content.Digests(), loaded.Content.Digests())
		be.NilErr(t, loaded.Content.Open(ctx, reg))
		be.NilErr(t, loaded.Content.Check(ctx))
		objFS := testutil.TmpLocalFS(t)
		be.NilErr(t, loaded.Update.Finalize("v1", ocfl.User{Name: "Tester"}))
		obj, err := loaded.Update.Apply(ctx, objFS, "obj", loaded.Content)
		be.NilErr(t, err)
		be.NilErr(t, ocfl.ValidateObject(ctx, objFS, obj.Path()).Err())
	})
	t.Run("content in memory can't be saved", func(t *testing.T) {
		upd, err := ocfl.NewUpdate(ctx, objFS, "obj2", "obj2")
		be.NilErr(t, err)
		stage := ocfl.NewStage(upd)
		be.NilErr(t, stage.AddBytes("a.txt", []byte("a")))
		_, err = json.Marshal(stage)
		be.Nonzero(t, err)
	})
	t.Run("invalid saved values", func(t *testing.T) {
		for name, data := range map[string]string{
			"missing content": `{"update":` + string(mustMarshal(t, upd)) + `}`,
			"missing update":  `{"content":{"sources":[],"content":{}}}`,
			"unknown field":   string(saved[:len(saved)-1]) + `,"extra":1}`,
		} {
			t.Run(name, func(t *testing.T) {
				var stage ocfl.Stage
				be.Nonzero(t, json.Unmarshal([]byte(data), &stage))
			})
		}
	})
}

func TestStage_JSON_missingContent(t *testing.T) {
	// a stage whose content directory has been removed can be loaded, changed,
	// saved and reverted: only opening its content fails.
	ctx := context.Background()
	contentFS := testutil.TmpLocalFS(t, filepath.Join(`testdata`, `content-fixture`))
	objFS := testutil.TmpLocalFS(t)
	upd, err := ocfl.NewUpdate(ctx, objFS, "obj", "obj")
	be.NilErr(t, err)
	stage := ocfl.NewStage(upd)
	be.NilErr(t, stage.AddFS(ctx, contentFS, "content-fixture", "."))
	be.NilErr(t, upd.Finalize("v1", ocfl.User{Name: "Tester"}))
	saved, err := json.Marshal(stage)
	be.NilErr(t, err)
	contentText, err := contentFS.MarshalText()
	be.NilErr(t, err)
	be.NilErr(t, contentFS.Close())
	be.NilErr(t, os.RemoveAll(contentFS.Root()))

	var loaded ocfl.Stage
	be.NilErr(t, json.Unmarshal(saved, &loaded))
	be.True(t, upd.State().Eq(loaded.Update.State()))
	be.True(t, loaded.Update.Finalized())
	resaved, err := json.Marshal(loaded)
	be.NilErr(t, err)
	be.Equal(t, string(saved), string(resaved))

	// content that was never opened is missing, so nothing is written
	_, err = loaded.Update.Apply(ctx, objFS, "obj", loaded.Content)
	be.True(t, errors.Is(err, ocfl.ErrMissingContent))
	_, err = ocflfs.ReadDir(ctx, objFS, "obj")
	be.True(t, errors.Is(err, fs.ErrNotExist))

	err = loaded.Content.Open(ctx, config.Registry())
	be.True(t, errors.Is(err, fs.ErrNotExist))
	be.In(t, string(contentText), err.Error())

	be.NilErr(t, loaded.Update.Revert(ctx, objFS, "obj"))
	be.False(t, loaded.Update.Finalized())
	be.NilErr(t, loaded.Remove("hello.csv"))
	_, err = json.Marshal(loaded)
	be.NilErr(t, err)
}
