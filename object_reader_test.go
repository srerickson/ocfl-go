package ocfl_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/srerickson/ocfl"
)

func TestObjectReader(t *testing.T) {
	obj, err := ocfl.NewObjectReader(os.DirFS(filepath.Join(goodObjPath, `spec-ex-full`)))
	if err != nil {
		t.Fatal(err)
	}
	if err := fstest.TestFS(obj, "v2/foo/bar.xml", "v3/empty2.txt"); err != nil {
		t.Error(err)
	}

	obj, err = ocfl.NewObjectReader(os.DirFS(filepath.Join(goodObjPath, `updates_three_versions_one_file`)))
	if err != nil {
		t.Fatal(err)
	}
	if err := fstest.TestFS(obj, "v1/a_file.txt", "v2/a_file.txt", "v3/a_file.txt"); err != nil {
		t.Error(err)
	}
}
