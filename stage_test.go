package ocfl_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
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

// fileStage is a stage whose content is in files, which can be saved as JSON.
type fileStage = ocfl.Stage[*ocfl.FileContentMap]

func TestUpdate_NeedsContent(t *testing.T) {
	ctx := context.Background()
	fixture := filepath.Join(objectFixturesPath, `1.1`, `good-objects`, `spec-ex-full`)
	obj, err := ocfl.NewObject(ctx, ocflfs.DirFS(fixture), ".")
	be.NilErr(t, err)
	u := obj.NewUpdate()
	inObj := u.NewState().DigestFor("empty2.txt")
	be.Nonzero(t, inObj)
	be.False(t, u.NeedsContent(inObj))
	be.Equal(t, 0, len(u.NewContent()))
	newDig := strings.Repeat("A", 128)
	be.NilErr(t, u.AddAll([]ocfl.UpdateEntry{
		{Name: "new/a.txt", Digest: newDig},
		{Name: "new/b.txt", Digest: newDig},
		{Name: "copy.txt", Digest: inObj},
	}))
	be.True(t, u.NeedsContent(newDig))
	be.DeepEqual(t, []string{strings.ToLower(newDig)}, u.NewContent())
	t.Run("AddAll adds nothing if there is an error", func(t *testing.T) {
		before := u.NewState()
		err := u.AddAll([]ocfl.UpdateEntry{
			{Name: "ok.txt", Digest: newDig},
			{Name: "new/a.txt/c.txt", Digest: newDig}, // conflicts with a file
		})
		be.Nonzero(t, err)
		be.True(t, before.Eq(u.NewState()))
	})
}

func TestStageBytes(t *testing.T) {
	ctx := context.Background()
	u, err := ocfl.NewUpdate(ctx, testutil.TmpLocalFS(t), "obj", "obj-1")
	be.NilErr(t, err)
	var mem ocfl.BytesContentMap
	be.NilErr(t, ocfl.StageBytes(u, &mem, "a.txt", []byte("a"), digest.MD5))
	dig := u.NewState().DigestFor("a.txt")
	be.DeepEqual(t, []string{dig}, mem.Digests())
	be.Nonzero(t, u.Fixity(dig)["md5"])
	srcFS, srcPath := mem.GetContent(strings.ToUpper(dig))
	got, err := ocflfs.ReadAll(ctx, srcFS, srcPath)
	be.NilErr(t, err)
	be.Equal(t, "a", string(got))
	be.NilErr(t, u.Finalize("v1", ocfl.User{Name: "Tester"}))
	be.True(t, errors.Is(ocfl.StageBytes(u, &mem, "b.txt", []byte("b")), ocfl.ErrFinalized))
	be.Equal(t, 1, len(mem.Digests()))
}

func TestMultiContentSource(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	writeOld(t, filepath.Join(dir, "a.txt"), "a")
	fsys, err := local.NewFS(dir)
	be.NilErr(t, err)
	digA := strings.Repeat("a", 128)
	digB := strings.Repeat("b", 128)
	var files ocfl.FileContentMap
	var mem ocfl.BytesContentMap
	info, err := ocflfs.StatFile(ctx, fsys, "a.txt")
	be.NilErr(t, err)
	files.AddFile(digA, fsys, "a.txt", info)
	mem.AddBytes(digB, []byte("b"))
	src := ocfl.MultiContentSource{&files, &mem}
	srcFS, srcPath := src.GetContent(digA)
	be.Equal[ocflfs.FS](t, fsys, srcFS)
	be.Equal(t, "a.txt", srcPath)
	srcFS, _ = src.GetContent(digB)
	be.Nonzero(t, srcFS)
	srcFS, _ = src.GetContent(strings.Repeat("c", 128))
	be.Zero(t, srcFS)
	be.NilErr(t, src.ContentFastCheck(ctx, []string{digA, digB}))
	// the changed file is found by the FileContentMap's check
	be.NilErr(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed"), 0o644))
	err = src.ContentFastCheck(ctx, []string{digA, digB})
	be.True(t, errors.Is(err, ocfl.ErrContentChanged))
}

func TestStageFS(t *testing.T) {
	ctx := context.Background()
	testdataFS := ocflfs.DirFS(`testdata`)
	newStage := func(t *testing.T) *testStage {
		stage, err := newTestStage(ctx, ocflfs.DirFS(t.TempDir()), "obj", "obj", ocfl.UpdateWithDigestAlgorithm(digest.SHA256))
		be.NilErr(t, err)
		return stage
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
		}, stage.Update().NewState().AllPaths())
		// every new digest has content and fixity
		state := stage.Update().NewState()
		be.Equal(t, 3, len(state))
		be.DeepEqual(t, sortedKeys(state), stage.files.Digests())
		for dig := range state {
			be.Nonzero(t, stage.Update().Fixity(dig)["md5"])
			srcFS, srcPath := stage.Content().GetContent(dig)
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
		}, stage.Update().NewState().AllPaths())
	})
	t.Run("conflicting paths add nothing", func(t *testing.T) {
		stage := newStage(t)
		be.NilErr(t, stage.AddBytes("folder1", []byte("a file, not a directory")))
		err := stage.AddFS(ctx, testdataFS, "content-fixture", ".")
		be.Nonzero(t, err)
		be.DeepEqual(t, []string{"folder1"}, stage.Update().NewState().AllPaths())
		be.Equal(t, 0, len(stage.files.Digests()))
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
		be.DeepEqual(t, []string{"a.txt", "dir/e/f.txt"}, stage.Update().NewState().AllPaths())
		stage = newStage(t)
		be.NilErr(t, stage.AddFS(ctx, hiddenFS, ".", ".", ocfl.StageWithHidden()))
		be.DeepEqual(t, []string{
			".b.txt",
			".dir/c.txt",
			"a.txt",
			"dir/.d.txt",
			"dir/e/f.txt",
		}, stage.Update().NewState().AllPaths())
		be.Equal(t, 5, len(stage.files.Digests()))
	})
	t.Run("hidden symlink adds nothing", func(t *testing.T) {
		// content-fixture's hidden directory has a symlink, which can't be
		// added
		stage := newStage(t)
		err := stage.AddFS(ctx, testdataFS, "content-fixture", ".", ocfl.StageWithHidden())
		be.True(t, errors.Is(err, ocflfs.ErrFileType))
		be.Equal(t, 0, len(stage.Update().NewState().AllPaths()))
		be.Equal(t, 0, len(stage.files.Digests()))
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
		}, stage.Update().NewState().AllPaths())
	})
	t.Run("go limit", func(t *testing.T) {
		serial := newStage(t)
		be.NilErr(t, serial.AddFS(ctx, testdataFS, "content-fixture", ".", ocfl.StageWithFixity(digest.MD5)))
		concurrent := newStage(t)
		be.NilErr(t, concurrent.AddFS(ctx, testdataFS, "content-fixture", ".",
			ocfl.StageWithFixity(digest.MD5), ocfl.StageWithGoLimit(4)))
		be.True(t, serial.Update().NewState().Eq(concurrent.Update().NewState()))
		be.DeepEqual(t, serial.files.Digests(), concurrent.files.Digests())
		for dig := range serial.Update().NewState() {
			be.DeepEqual(t, serial.Update().Fixity(dig), concurrent.Update().Fixity(dig))
		}
	})
	t.Run("digest error adds nothing", func(t *testing.T) {
		badFS := &openErrFS{WrapFS: ocflfs.DirFS(`testdata`), name: "content-fixture/hello.csv"}
		stage := newStage(t)
		err := stage.AddFS(ctx, badFS, "content-fixture", ".", ocfl.StageWithGoLimit(4))
		be.True(t, errors.Is(err, errOpen))
		be.Equal(t, 0, len(stage.Update().NewState().AllPaths()))
		be.Equal(t, 0, len(stage.files.Digests()))
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

func TestStageFile(t *testing.T) {
	ctx := context.Background()
	testdataFS := ocflfs.DirFS(`testdata`)
	stage, err := newTestStage(ctx, ocflfs.DirFS(t.TempDir()), "obj", "obj")
	be.NilErr(t, err)
	upd := stage.Update()
	be.NilErr(t, stage.AddFile(ctx, testdataFS, "content-fixture/hello.csv", "data/hello.csv", ocfl.StageWithFixity(digest.SIZE)))
	dig := upd.NewState().DigestFor("data/hello.csv")
	be.Nonzero(t, dig)
	be.Nonzero(t, upd.Fixity(dig)["size"])
	srcFS, srcPath := stage.Content().GetContent(dig)
	be.Equal[ocflfs.FS](t, testdataFS, srcFS)
	be.Equal(t, "content-fixture/hello.csv", srcPath)
	t.Run("missing file", func(t *testing.T) {
		err := stage.AddFile(ctx, testdataFS, "missing", "missing")
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})
	t.Run("hidden file", func(t *testing.T) {
		name := "content-fixture/folder1/folder2/.hidden_file.txt"
		be.NilErr(t, stage.AddFile(ctx, testdataFS, name, ".hidden.txt"))
		be.Nonzero(t, upd.NewState().DigestFor(".hidden.txt"))
	})
}

func TestStage_Edits(t *testing.T) {
	// content isn't removed when the update stops needing it: it does no
	// harm, and DeleteFunc with NeedsContent removes it.
	ctx := context.Background()
	fixture := filepath.Join(objectFixturesPath, `1.1`, `good-objects`, `spec-ex-full`)
	obj, err := ocfl.NewObject(ctx, ocflfs.DirFS(fixture), ".")
	be.NilErr(t, err)
	prune := func(stage *testStage) {
		unneeded := func(dig string) bool { return !stage.NeedsContent(dig) }
		stage.files.DeleteFunc(unneeded)
		stage.mem.DeleteFunc(unneeded)
	}
	t.Run("content already in the object", func(t *testing.T) {
		stage := stageFor(obj.NewUpdate())
		// empty2.txt's content is in the object: only the state changes
		be.NilErr(t, stage.AddBytes("empty3.txt", []byte{}, digest.MD5))
		be.Equal(t, stage.Update().NewState().DigestFor("empty2.txt"), stage.Update().NewState().DigestFor("empty3.txt"))
		be.Equal(t, 0, len(stage.mem.Digests()))
		be.Zero(t, stage.Update().Fixity(stage.Update().NewState().DigestFor("empty3.txt")))
	})
	t.Run("remove drops fixity", func(t *testing.T) {
		stage := stageFor(obj.NewUpdate())
		be.NilErr(t, stage.AddBytes("new/a.txt", []byte("a"), digest.MD5))
		be.NilErr(t, stage.AddBytes("new/b.txt", []byte("a")))
		dig := stage.Update().NewState().DigestFor("new/a.txt")
		be.NilErr(t, stage.Remove("new/a.txt"))
		// content is still used by new/b.txt
		prune(stage)
		be.DeepEqual(t, []string{dig}, stage.mem.Digests())
		be.Nonzero(t, stage.Update().Fixity(dig))
		be.NilErr(t, stage.Remove("new"))
		be.Zero(t, stage.Update().Fixity(dig))
		be.Equal(t, 0, len(stage.NewContent()))
		be.Equal(t, 1, len(stage.mem.Digests()))
		prune(stage)
		be.Equal(t, 0, len(stage.mem.Digests()))
		err := stage.Remove("new")
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})
	t.Run("clear", func(t *testing.T) {
		stage := stageFor(obj.NewUpdate())
		be.NilErr(t, stage.AddBytes("new/a.txt", []byte("a"), digest.MD5))
		be.NilErr(t, stage.AddFS(ctx, ocflfs.DirFS(`testdata`), "content-fixture", "fixture"))
		be.NilErr(t, stage.Clear())
		be.Equal(t, 0, len(stage.Update().NewState()))
		prune(stage)
		be.Equal(t, 0, len(stage.files.Digests()))
		be.Equal(t, 0, len(stage.mem.Digests()))
		// the content can be added again
		be.NilErr(t, stage.AddBytes("new/a.txt", []byte("a")))
		be.Equal(t, 1, len(stage.mem.Digests()))
	})
	t.Run("rename keeps content", func(t *testing.T) {
		stage := stageFor(obj.NewUpdate())
		be.NilErr(t, stage.AddBytes("new/a.txt", []byte("a")))
		be.NilErr(t, stage.Rename("new", "newer"))
		dig := stage.Update().NewState().DigestFor("newer/a.txt")
		be.Nonzero(t, dig)
		be.DeepEqual(t, []string{dig}, stage.mem.Digests())
	})
}

func TestStage_JSON(t *testing.T) {
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
	obj, err := loaded.Apply(ctx, objFS, "obj")
	be.NilErr(t, err)
	be.NilErr(t, ocfl.ValidateObject(ctx, objFS, obj.Path()).Err())
	be.DeepEqual(t, []string{"md5"}, obj.FixityAlgorithms())

	t.Run("file sizes and tokens", func(t *testing.T) {
		var savedJSON struct {
			Content struct{ Content map[string][]any }
		}
		be.NilErr(t, json.Unmarshal(saved, &savedJSON))
		be.Equal(t, 3, len(savedJSON.Content.Content))
		for _, entry := range savedJSON.Content.Content {
			be.Equal(t, 4, len(entry))
			info, err := os.Stat(filepath.Join(contentFS.Root(), entry[1].(string)))
			be.NilErr(t, err)
			be.Equal(t, float64(info.Size()), entry[2].(float64))
			be.Equal(t, ocflfs.ContentToken(info), entry[3].(string))
		}
	})
	t.Run("saved without file sizes or tokens", func(t *testing.T) {
		// stages whose sizes and tokens are unknown can be loaded and
		// applied
		var old map[string]any
		be.NilErr(t, json.Unmarshal(saved, &old))
		content := old["content"].(map[string]any)["content"].(map[string]any)
		for dig, entry := range content {
			content[dig] = append(entry.([]any)[:2], nil, nil)
		}
		var loaded fileStage
		be.NilErr(t, json.Unmarshal(mustMarshal(t, old), &loaded))
		be.DeepEqual(t, stage.Content.Digests(), loaded.Content.Digests())
		be.NilErr(t, loaded.Content.OpenFS(ctx, reg))
		be.NilErr(t, loaded.Content.FastCheck(ctx))
		objFS := testutil.TmpLocalFS(t)
		be.NilErr(t, loaded.Update.Finalize("v1", ocfl.User{Name: "Tester"}))
		obj, err := loaded.Apply(ctx, objFS, "obj")
		be.NilErr(t, err)
		be.NilErr(t, ocfl.ValidateObject(ctx, objFS, obj.Path()).Err())
	})
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

func TestStage_JSON_missingContent(t *testing.T) {
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
