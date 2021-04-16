package ocfl_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/srerickson/ocfl"
)

var fixturePath = filepath.Join(`test`, `fixtures`, `1.0`)
var goodObjPath = filepath.Join(fixturePath, `good-objects`)
var badObjPath = filepath.Join(fixturePath, `bad-objects`)
var warnObjPath = filepath.Join(fixturePath, `warn-objects`)

func TestNewObjectFS(t *testing.T) {
	root := os.DirFS(filepath.Join(goodObjPath, `spec-ex-full`))
	obj, err := ocfl.NewObjectReader(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := fstest.TestFS(obj, `v1/foo/bar.xml`); err != nil {
		t.Fatal(err)
	}
	file, err := obj.Open(`v1/foo/bar.xml`)
	if err != nil {
		t.Fatal(err)
	}
	data, err := ioutil.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	expected := "Me, Myself, I"
	if !strings.Contains(string(data), expected) {
		t.Fatalf("expected file to contain %s", expected)
	}
}

func TestValidateObject(t *testing.T) {
	goodObj := filepath.Join(goodObjPath, `spec-ex-full`)
	badObj := filepath.Join(badObjPath, `E003_no_decl`)
	result := ocfl.ValidateObject(os.DirFS(goodObj))
	if !result.Valid() {
		t.Errorf("expected %s to be valid", goodObj)
		for _, err := range result.Fatal() {
			t.Errorf(`--> %s`, err.Error())
		}
	}
	if !ocfl.ValidateObject(os.DirFS(badObj)).Valid() {
		t.Errorf("expected %s to be invalid", badObj)

	}
	if ocfl.ValidateObject(nil).Valid() {
		t.Errorf("expected %s to be invalid", badObj)
	}
}
