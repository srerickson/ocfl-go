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
		t.Error(err)
	}
	if err := fstest.TestFS(obj, "v2/foo/bar.xml", "v3/empty2.txt"); err != nil {
		t.Error(err)
	}

}
