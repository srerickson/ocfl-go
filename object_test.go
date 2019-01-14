package ocfl

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestNewObject(t *testing.T) {
	user := NewUser(`tester`, `tester@nowhere`)
	objectRoot, err := ioutil.TempDir(`./`, `test-object`)
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(objectRoot)

	object, err := InitObject(objectRoot, `test-object-id`)
	if err != nil {
		t.Error(err)
	}

	// version 1
	stage, err := object.NewStage()
	if err != nil {
		t.Error(err)
	}
	path1 := filepath.Join(`dir`, `test-1.txt`)
	file, err := stage.OpenFile(path1)
	if err != nil {
		t.Error(err)
	}
	_, err = file.WriteString(`testing testing`)
	if err != nil {
		t.Error(err)
	}
	file.Close()
	if err := stage.Rename(path1, `test-2.txt`); err != nil {
		t.Error(err)
	}
	if err = stage.Commit(user, `commit version 1`); err != nil {
		t.Error(err)
	}
	ePath := filepath.Join(`v1`, `content`, `test-2.txt`)
	if object.inventory.Manifest.GetDigest(Path(ePath)) == `` {
		t.Errorf(`expected %s to exist in manifest`, ePath)
	}
	if err = ValidateObject(objectRoot); err != nil {
		t.Error(err)
	}

	// version 2
	stage, err = object.NewStage()
	if err != nil {
		t.Error(err)
	}
	path2 := filepath.Join(`dir`, `test-3.txt`)
	file, err = stage.OpenFile(path2)
	if err != nil {
		t.Error(err)
	}
	_, err = file.WriteString(`words words words`)
	if err != nil {
		t.Error(err)
	}
	file.Close()
	if err = stage.Remove(`test-2.txt`); err != nil {
		t.Error(err)
	}
	if err = stage.Commit(user, `commit version 2`); err != nil {
		t.Error(err)
	}
	if err = ValidateObject(objectRoot); err != nil {
		t.Error(err)
	}
}
