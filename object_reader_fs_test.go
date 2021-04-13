package ocfl_test

import (
	"io/fs"
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
	info, err := fs.Stat(obj, "v3/empty2.txt")
	if err != nil {
		t.Error(err)
	}
	t.Log(info.Mode().Type())
	f, _ := obj.Open("v3/empty2.txt")
	info, _ = f.Stat()
	t.Log(info.Mode().Type())

	if err := fstest.TestFS(obj, "v2/foo/bar.xml"); err != nil {
		t.Error(err)
	}

}
