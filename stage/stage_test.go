package stage_test

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/internal/testutil"
	"github.com/srerickson/ocfl-go/stage"
)

func TestStageBytes(t *testing.T) {
	ctx := context.Background()
	u, _, err := ocfl.NewObjectUpdate(ctx, testutil.TmpLocalFS(t), "obj", "obj-1")
	be.NilErr(t, err)
	var mem stage.BytesContentMap
	be.NilErr(t, stage.Bytes(u, &mem, "a.txt", []byte("a"), digest.MD5))
	dig := u.NewState().DigestFor("a.txt")
	be.DeepEqual(t, []string{dig}, mem.Digests())
	be.Nonzero(t, u.Fixity(dig)["md5"])
	srcFS, srcPath := mem.GetContent(strings.ToUpper(dig))
	got, err := ocflfs.ReadAll(ctx, srcFS, srcPath)
	be.NilErr(t, err)
	be.Equal(t, "a", string(got))
	be.NilErr(t, u.Finalize("v1", ocfl.User{Name: "Tester"}))
	be.True(t, errors.Is(stage.Bytes(u, &mem, "b.txt", []byte("b")), ocfl.ErrFinalized))
	be.Equal(t, 1, len(mem.Digests()))
	t.Run("content already in the object", func(t *testing.T) {
		fixture := filepath.Join(objectFixturesPath, `1.1`, `good-objects`, `spec-ex-full`)
		u, _, err := ocfl.NewObjectUpdate(ctx, ocflfs.DirFS(fixture), ".", "")
		be.NilErr(t, err)
		var mem stage.BytesContentMap
		// empty2.txt's content is in the object: only the state changes
		be.NilErr(t, stage.Bytes(u, &mem, "empty3.txt", []byte{}, digest.MD5))
		dig := u.NewState().DigestFor("empty3.txt")
		be.Equal(t, u.NewState().DigestFor("empty2.txt"), dig)
		be.Equal(t, 0, len(mem.Digests()))
		be.Zero(t, u.Fixity(dig))
	})
	t.Run("nil content panics before u is changed", func(t *testing.T) {
		u, _, err := ocfl.NewObjectUpdate(ctx, testutil.TmpLocalFS(t), "obj", "obj-1")
		be.NilErr(t, err)
		defer func() {
			be.Nonzero(t, recover())
			be.Equal(t, 0, len(u.NewState()))
		}()
		stage.Bytes(u, nil, "a.txt", []byte("a"))
	})
}

func TestStageFS(t *testing.T) {
	ctx := context.Background()
	testdataFS := ocflfs.DirFS(testdataPath)
	newStage := func(t *testing.T) *testStage {
		stg, err := newTestStage(ctx, ocflfs.DirFS(t.TempDir()), "obj", "obj", ocfl.NewUpdateWithDigestAlgorithm(digest.SHA256))
		be.NilErr(t, err)
		return stg
	}
	t.Run("top-level directory", func(t *testing.T) {
		stg := newStage(t)
		be.NilErr(t, stg.AddFS(ctx, testdataFS, "content-fixture", ".", stage.WithFixity(digest.MD5)))
		// hidden files are ignored
		be.DeepEqual(t, []string{
			"folder1/file.txt",
			"folder1/folder2/file2.txt",
			"folder1/folder2/sculpture-stone-face-head-888027.jpg",
			"hello.csv",
		}, stg.Update().NewState().AllPaths())
		// every new digest has content and fixity
		state := stg.Update().NewState()
		be.Equal(t, 3, len(state))
		be.DeepEqual(t, stg.Update().NewContentDigests(), stg.files.Digests())
		for dig := range state {
			be.Nonzero(t, stg.Update().Fixity(dig)["md5"])
			srcFS, srcPath := stg.Content().GetContent(dig)
			be.Nonzero(t, srcFS)
			_, err := ocflfs.StatFile(ctx, srcFS, srcPath)
			be.NilErr(t, err)
		}
	})
	t.Run("subdirectory", func(t *testing.T) {
		stg := newStage(t)
		be.NilErr(t, stg.AddFS(ctx, testdataFS, "content-fixture/folder1", "a/b"))
		be.DeepEqual(t, []string{
			"a/b/file.txt",
			"a/b/folder2/file2.txt",
			"a/b/folder2/sculpture-stone-face-head-888027.jpg",
		}, stg.Update().NewState().AllPaths())
	})
	t.Run("conflicting paths add nothing", func(t *testing.T) {
		stg := newStage(t)
		be.NilErr(t, stg.AddBytes("folder1", []byte("a file, not a directory")))
		err := stg.AddFS(ctx, testdataFS, "content-fixture", ".")
		be.Nonzero(t, err)
		be.DeepEqual(t, []string{"folder1"}, stg.Update().NewState().AllPaths())
		be.Equal(t, 0, len(stg.files.Digests()))
	})
	t.Run("missing directory", func(t *testing.T) {
		stg := newStage(t)
		err := stg.AddFS(ctx, testdataFS, "missing", ".")
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
		stg := newStage(t)
		be.NilErr(t, stg.AddFS(ctx, hiddenFS, ".", "."))
		be.DeepEqual(t, []string{"a.txt", "dir/e/f.txt"}, stg.Update().NewState().AllPaths())
		stg = newStage(t)
		be.NilErr(t, stg.AddFS(ctx, hiddenFS, ".", ".", stage.WithHidden()))
		be.DeepEqual(t, []string{
			".b.txt",
			".dir/c.txt",
			"a.txt",
			"dir/.d.txt",
			"dir/e/f.txt",
		}, stg.Update().NewState().AllPaths())
		be.Equal(t, 5, len(stg.files.Digests()))
	})
	t.Run("hidden symlink adds nothing", func(t *testing.T) {
		// content-fixture's hidden directory has a symlink, which can't be
		// added
		stg := newStage(t)
		err := stg.AddFS(ctx, testdataFS, "content-fixture", ".", stage.WithHidden())
		be.True(t, errors.Is(err, ocflfs.ErrFileType))
		be.Equal(t, 0, len(stg.Update().NewState().AllPaths()))
		be.Equal(t, 0, len(stg.files.Digests()))
	})
	t.Run("filter", func(t *testing.T) {
		stg := newStage(t)
		noHiddenDir := func(ref *ocflfs.FileRef) bool { return path.Base(path.Dir(ref.Path)) != ".hidden_folder" }
		be.NilErr(t, stg.AddFS(ctx, testdataFS, "content-fixture/folder1", ".", stage.WithFilter(noHiddenDir)))
		// the filter replaces the default, so hidden files that pass it are
		// added
		be.DeepEqual(t, []string{
			"file.txt",
			"folder2/.hidden_file.txt",
			"folder2/file2.txt",
			"folder2/sculpture-stone-face-head-888027.jpg",
		}, stg.Update().NewState().AllPaths())
	})
	t.Run("go limit", func(t *testing.T) {
		serial := newStage(t)
		be.NilErr(t, serial.AddFS(ctx, testdataFS, "content-fixture", ".", stage.WithFixity(digest.MD5)))
		concurrent := newStage(t)
		be.NilErr(t, concurrent.AddFS(ctx, testdataFS, "content-fixture", ".",
			stage.WithFixity(digest.MD5), stage.WithGoLimit(4)))
		be.True(t, serial.Update().NewState().Eq(concurrent.Update().NewState()))
		be.DeepEqual(t, serial.files.Digests(), concurrent.files.Digests())
		for dig := range serial.Update().NewState() {
			be.DeepEqual(t, serial.Update().Fixity(dig), concurrent.Update().Fixity(dig))
		}
	})
	t.Run("digest error adds nothing", func(t *testing.T) {
		badFS := &openErrFS{WrapFS: ocflfs.DirFS(testdataPath), name: "content-fixture/hello.csv"}
		stg := newStage(t)
		err := stg.AddFS(ctx, badFS, "content-fixture", ".", stage.WithGoLimit(4))
		be.True(t, errors.Is(err, errOpen))
		be.Equal(t, 0, len(stg.Update().NewState().AllPaths()))
		be.Equal(t, 0, len(stg.files.Digests()))
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
	testdataFS := ocflfs.DirFS(testdataPath)
	stg, err := newTestStage(ctx, ocflfs.DirFS(t.TempDir()), "obj", "obj")
	be.NilErr(t, err)
	upd := stg.Update()
	be.NilErr(t, stg.AddFile(ctx, testdataFS, "content-fixture/hello.csv", "data/hello.csv", stage.WithFixity(digest.SIZE)))
	dig := upd.NewState().DigestFor("data/hello.csv")
	be.Nonzero(t, dig)
	be.Nonzero(t, upd.Fixity(dig)["size"])
	srcFS, srcPath := stg.Content().GetContent(dig)
	be.Equal[ocflfs.FS](t, testdataFS, srcFS)
	be.Equal(t, "content-fixture/hello.csv", srcPath)
	t.Run("missing file", func(t *testing.T) {
		err := stg.AddFile(ctx, testdataFS, "missing", "missing")
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})
	t.Run("hidden file", func(t *testing.T) {
		name := "content-fixture/folder1/folder2/.hidden_file.txt"
		be.NilErr(t, stg.AddFile(ctx, testdataFS, name, ".hidden.txt"))
		be.Nonzero(t, upd.NewState().DigestFor(".hidden.txt"))
	})
}
