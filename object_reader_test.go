package ocfl_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/srerickson/ocfl"
)

//var warnObjPath = filepath.Join(fixturePath, `warn-objects`)

func TestObjectReader(t *testing.T) {
	obj, err := ocfl.NewObjectReader(os.DirFS(filepath.Join(goodObjPath, `spec-ex-full`)))
	if err != nil {
		t.Fatal(err)
	}
	err = obj.Validate()
	if err != nil {
		t.Fatal(err)
	}
	v2, err := obj.VersionFS(`v2`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = v2.Open(`foo/bar.xml`)
	if err != nil {
		t.Error(err)
	}
	v3, err := obj.VersionFS(`v3`)
	if err != nil {
		t.Error(err)
	}
	_, err = v3.Open(`not found`)
	if err == nil {
		t.Error(`expected an error`)
	}
}
