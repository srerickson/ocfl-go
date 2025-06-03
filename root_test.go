package ocfl_test

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/extension"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/local"
)

func TestNewRoot(t *testing.T) {
	ctx := context.Background()
	t.Run("fixture reg-extension-dir-root", func(t *testing.T) {
		fsys := ocflfs.DirFS(storeFixturePath)
		dir := `1.0/good-stores/reg-extension-dir-root`
		root, err := ocfl.NewRoot(ctx, fsys, dir)
		be.NilErr(t, err)
		be.Equal(t, ocfl.Spec1_0, root.Spec())
		obj, err := root.NewObject(ctx, "ark:123/abc")
		be.NilErr(t, err)
		be.True(t, obj.Exists())
	})
	t.Run("fixture simple-root", func(t *testing.T) {
		fsys := ocflfs.DirFS(storeFixturePath)
		dir := `1.0/good-stores/simple-root`
		root, err := ocfl.NewRoot(ctx, fsys, dir)
		be.NilErr(t, err)
		be.Equal(t, ocfl.Spec1_0, root.Spec())
	})

}

func TestRoot_Example(t *testing.T) {
	ctx := context.Background()
	fsys, err := local.NewFS(t.TempDir())
	be.NilErr(t, err)
	// new root settings
	dir := `new-root`
	desc := "a new root"
	layout := extension.Ext0004()
	newRoot, err := ocfl.NewRoot(ctx, fsys, dir, ocfl.InitRoot(ocfl.Spec1_1, desc, layout))
	be.NilErr(t, err)
	be.Equal(t, layout.Name(), newRoot.Layout().Name())
	be.Equal(t, ocfl.Spec1_1, newRoot.Spec())
	be.Equal(t, desc, newRoot.Description())
	// update an object
	objID := "object-1"
	obj, err := newRoot.NewObject(ctx, objID)
	be.NilErr(t, err)
	be.Equal(t, obj.ID(), objID)
	stage, err := ocfl.StageBytes(map[string][]byte{
		"file.txt": []byte("readme readme readme"),
	}, digest.SHA256)
	be.NilErr(t, err)
	_, err = obj.Update(ctx, stage, "first version", ocfl.User{Name: "Stinky & Dirty"})
	be.NilErr(t, err)
	// re-open and validate object
	sameRoot, err := ocfl.NewRoot(ctx, fsys, dir)
	be.NilErr(t, err)
	be.Equal(t, layout.Name(), sameRoot.Layout().Name())
	be.Equal(t, ocfl.Spec1_1, sameRoot.Spec())
	be.Equal(t, desc, sameRoot.Description())
	sameObj, err := sameRoot.NewObject(ctx, objID)
	be.NilErr(t, err)
	be.NilErr(t, ocfl.ValidateObject(ctx, obj.FS(), obj.Path()).Err())
	be.Equal(t, objID, sameObj.ID())
}

func TestRoot_ObjectsBatch(t *testing.T) {
	ctx := context.Background()
	fsys := ocflfs.DirFS(filepath.Join(`testdata`, `store-fixtures`))

	t.Run("simple-root", func(t *testing.T) {
		dir := `1.0/good-stores/simple-root`
		root, err := ocfl.NewRoot(ctx, fsys, dir)
		be.NilErr(t, err)
		count := 0
		for obj, err := range root.ObjectsBatch(ctx, 2) {
			be.NilErr(t, err)
			count++
			be.True(t, obj.Exists())
			be.Equal(t, root, obj.Root())
		}
		be.Equal(t, 3, count)
	})

	t.Run("break iteration", func(t *testing.T) {
		dir := `1.0/good-stores/simple-root`
		root, err := ocfl.NewRoot(ctx, fsys, dir)
		be.NilErr(t, err)
		// check that iterator doesn't after break: this will panic
		defer func() {
			if err := recover(); err != nil {
				t.Fatal(err)
			}
		}()
		for range root.ObjectsBatch(ctx, 1) {
			break
		}
	})

	t.Run("root with error", func(t *testing.T) {
		dir := `1.0/bad-stores/multi_level_errors`
		root, err := ocfl.NewRoot(ctx, fsys, dir)
		be.NilErr(t, err)
		count := 0
		// iterate over all declarations, even if there are errors
		for range root.ObjectsBatch(ctx, 1) {
			count++
		}
		be.Equal(t, 3, count)
	})

}

func TestRoot_ObjectDeclarations(t *testing.T) {
	ctx := context.Background()
	fsys := ocflfs.DirFS(filepath.Join(`testdata`, `store-fixtures`))
	dir := `1.0/good-stores/simple-root`
	root, err := ocfl.NewRoot(ctx, fsys, dir)
	be.NilErr(t, err)

	t.Run("simple-root", func(t *testing.T) {
		count := 0
		for ref, err := range root.ObjectDeclarations(ctx) {
			be.NilErr(t, err)
			be.Nonzero(t, ref.Info)
			count++
		}
		be.Equal(t, 3, count)
	})

	t.Run("break iteration", func(t *testing.T) {
		// check that iterator doesn't after break: this will panic
		defer func() {
			if err := recover(); err != nil {
				t.Fatal(err)
			}
		}()
		for range root.ObjectDeclarations(ctx) {
			break
		}
	})
}

func TestRoot_ValidateObject(t *testing.T) {
	ctx := context.Background()
	fsys := ocflfs.DirFS(filepath.Join(`testdata`, `store-fixtures`))
	dir := `1.0/good-stores/simple-root`
	root, err := ocfl.NewRoot(ctx, fsys, dir)
	be.NilErr(t, err)
	t.Run("simple", func(t *testing.T) {
		objPath := "http%3A%2F%2Fexample.org%2Fminimal_mixed_digests"
		valid := root.ValidateObjectDir(ctx, objPath)
		be.NilErr(t, valid.Err())
	})
	t.Run("not exist", func(t *testing.T) {
		objPath := "none"
		err = root.ValidateObjectDir(ctx, objPath).Err()
		be.True(t, err != nil)
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})
}
