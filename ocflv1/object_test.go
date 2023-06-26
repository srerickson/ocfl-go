package ocflv1_test

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/ocflv1"
)

var fixturePath = filepath.Join(`..`, `testdata`, `object-fixtures`, `1.1`)
var goodObjPath = filepath.Join(fixturePath, `good-objects`)

//var warnObjPath = filepath.Join(fixturePath, `warn-objects`)
//var badObjPath = filepath.Join(fixturePath, `bad-objects`)

func TestReadObject(t *testing.T) {
	fsys := ocfl.NewFS(os.DirFS(goodObjPath))
	obj, err := ocflv1.GetObject(context.Background(), fsys, "spec-ex-full")
	if err != nil {
		t.Fatal(err)
	}
	vnums := obj.Inventory.VNums()
	if len(vnums) != 3 {
		t.Error("expected 3 versions")
	}
	if obj.Inventory.Head.Num() != 3 {
		t.Error("expected head to be version 3")
	}
	cont, err := obj.Inventory.ContentPath(0, "foo/bar.xml")
	if err != nil {
		t.Error(err)
	}
	f, err := fsys.OpenFile(context.Background(), path.Join("spec-ex-full", cont))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}
