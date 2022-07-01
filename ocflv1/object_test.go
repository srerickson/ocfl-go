package ocflv1_test

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/srerickson/ocfl/object"
	"github.com/srerickson/ocfl/ocflv1"
)

var fixturePath = filepath.Join(`testdata`, `object-fixtures`, `1.0`)
var goodObjPath = filepath.Join(fixturePath, `good-objects`)

//var badObjPath = filepath.Join(fixturePath, `bad-objects`)

func TestReadObject(t *testing.T) {
	fsys := os.DirFS(goodObjPath)
	obj, err := ocflv1.GetObject(context.Background(), fsys, "spec-ex-full")
	if err != nil {
		t.Fatal(err)
	}
	vnums, err := obj.Versions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(vnums) != 3 {
		t.Error("expected 3 versions")
	}
	head, err := obj.Head(context.Background())
	if err != nil {
		t.Error(err)
	}
	if head.Num() != 3 {
		t.Error("expected head to be version 3")
	}
	cont, err := obj.GetContent(context.Background(), object.VNum{}, "foo/bar.xml")
	if err != nil {
		t.Error(err)
	}
	_, err = fs.Stat(fsys, path.Join("spec-ex-full", cont))
	if err != nil {
		t.Fatal(err)
	}
}
