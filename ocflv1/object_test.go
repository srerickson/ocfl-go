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
// var _ ocfl.ContentSource = (*ocflv1.Object)(nil)
// var _ ocfl.FixitySource = (*ocflv1.FunObject)(nil)

func TestReadObject(t *testing.T) {
	ctx := context.Background()
	fsys := ocfl.NewFS(os.DirFS(goodObjectsPath))
	goodObjName := "spec-ex-full"

	t.Run("ok", func(t *testing.T) {
		obj, err := ocflv1.OpenObject(ctx, fsys, goodObjName)
		be.NilErr(t, err)
		be.NilErr(t, obj.Close())
	})

}
