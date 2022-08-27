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

var fixturePath = filepath.Join(`..`, `testdata`, `object-fixtures`, `1.0`)
var goodObjPath = filepath.Join(fixturePath, `good-objects`)

//var badObjPath = filepath.Join(fixturePath, `bad-objects`)

func TestReadObject(t *testing.T) {
	fsys := ocfl.NewFS(os.DirFS(goodObjPath))
	obj, err := ocflv1.GetObject(context.Background(), fsys, "spec-ex-full")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := obj.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	vnums := inv.VNums()
	if len(vnums) != 3 {
		t.Error("expected 3 versions")
	}
	if inv.Head.Num() != 3 {
		t.Error("expected head to be version 3")
	}
	cont, err := inv.ContentPath(ocfl.VNum{}, "foo/bar.xml")
	if err != nil {
		t.Error(err)
	}
	f, err := fsys.OpenFile(context.Background(), path.Join("spec-ex-full", cont))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}
