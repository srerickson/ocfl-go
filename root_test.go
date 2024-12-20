package ocfl_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/local"
	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/extension"
)

func TestRoot(t *testing.T) {
	ctx := context.Background()

	t.Run("fixture reg-extension-dir-root", func(t *testing.T) {
		fsys := ocfl.DirFS(storeFixturePath)
		dir := `1.0/good-stores/reg-extension-dir-root`
		root, err := ocfl.NewRoot(ctx, fsys, dir)
		be.NilErr(t, err)
		be.Equal(t, ocfl.Spec1_0, root.Spec())
		obj, err := root.NewObject(ctx, "ark:123/abc")
		be.NilErr(t, err)
		be.True(t, obj.Exists())
	})

	t.Run("fixture simple-root", func(t *testing.T) {
		fsys := ocfl.DirFS(storeFixturePath)
		dir := `1.0/good-stores/simple-root`
		root, err := ocfl.NewRoot(ctx, fsys, dir)
		be.NilErr(t, err)
		be.Equal(t, ocfl.Spec1_0, root.Spec())
	})

	t.Run("init root and commit", func(t *testing.T) {
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

		// commit an object
		objID := "object-1"
		obj, err := newRoot.NewObject(ctx, objID)
		be.NilErr(t, err)
		stage, err := ocfl.StageBytes(map[string][]byte{
			"file.txt": []byte("readme readme readme"),
		}, digest.SHA256)
		be.NilErr(t, err)
		err = obj.Commit(ctx, &ocfl.Commit{
			Stage:   stage,
			Message: "first version",
			User:    ocfl.User{Name: "Stinky & Dirty"},
		})
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
		be.Equal(t, objID, sameObj.Inventory().ID())
	})
	t.Run("Objects", func(t *testing.T) {
		t.Run("simple-root", func(t *testing.T) {
			fsys := ocfl.DirFS(storeFixturePath)
			dir := `1.0/good-stores/simple-root`
			root, err := ocfl.NewRoot(ctx, fsys, dir)
			be.NilErr(t, err)
			count := 0
			for obj, err := range root.Objects(ctx) {
				be.NilErr(t, err)
				count++
				be.True(t, obj.Exists())
			}
			be.Equal(t, 3, count)
		})
	})

	t.Run("ObjectsBatch", func(t *testing.T) {
		t.Run("simple-root", func(t *testing.T) {
			fsys := ocfl.DirFS(storeFixturePath)
			dir := `1.0/good-stores/simple-root`
			root, err := ocfl.NewRoot(ctx, fsys, dir)
			be.NilErr(t, err)
			count := 0
			for obj, err := range root.ObjectsBatch(ctx, 2) {
				be.NilErr(t, err)
				count++
				be.True(t, obj.Exists())
			}
			be.Equal(t, 3, count)
		})
	})

	t.Run("ValidateObjectDir", func(t *testing.T) {
		t.Run("simple", func(t *testing.T) {
			fsys := ocfl.DirFS(storeFixturePath)
			dir := `1.0/good-stores/simple-root`
			root, err := ocfl.NewRoot(ctx, fsys, dir)
			be.NilErr(t, err)
			objPath := "http%3A%2F%2Fexample.org%2Fminimal_mixed_digests"
			valid := root.ValidateObjectDir(ctx, objPath)
			be.NilErr(t, valid.Err())
		})
		t.Run("missingDir", func(t *testing.T) {
			fsys := ocfl.DirFS(storeFixturePath)
			dir := `1.0/good-stores/simple-root`
			root, err := ocfl.NewRoot(ctx, fsys, dir)
			be.NilErr(t, err)
			objPath := "none"
			err = root.ValidateObjectDir(ctx, objPath).Err()
			be.True(t, err != nil)
			be.True(t, errors.Is(err, fs.ErrNotExist))
		})
	})
}
