package ocflv1_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1"
)

var fixturePath = filepath.Join(`..`, `testdata`, `object-fixtures`, `1.1`)
var goodObjectsPath = filepath.Join(fixturePath, `good-objects`)

//var warnObjPath = filepath.Join(fixturePath, `warn-objects`)
//var badObjPath = filepath.Join(fixturePath, `bad-objects`)

// object implements ocfl.ContentProvider and ocfl.FixityProvider
var _ ocfl.ContentSource = (*ocflv1.Object)(nil)
var _ ocfl.FixitySource = (*ocflv1.Object)(nil)

func TestReadObject(t *testing.T) {
	ctx := context.Background()
	fsys := ocfl.NewFS(os.DirFS(goodObjectsPath))
	goodObjName := "spec-ex-full"

	t.Run("ok", func(t *testing.T) {
		obj, err := ocflv1.GetObject(ctx, fsys, goodObjName)
		be.NilErr(t, err)
		be.Equal(t, 3, len(obj.Inventory.VNums()))
		be.Equal(t, 3, obj.Inventory.Head.Num())
		cont, err := obj.Inventory.ContentPath(0, "foo/bar.xml")
		be.NilErr(t, err)
		f, err := obj.OpenFile(ctx, cont)
		be.NilErr(t, err)
		defer f.Close()
	})

}
