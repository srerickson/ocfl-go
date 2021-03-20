package ocfl_test

import (
	"os"
	"testing"

	"github.com/srerickson/ocfl"
)

func TestObjectReader(t *testing.T) {
	obj, err := ocfl.NewObjectReader(os.DirFS(`test/fixtures/1.0/good-objects/spec-ex-full`))
	if err != nil {
		t.Fatal(err)
	}
	err = obj.CUEValudate()
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
