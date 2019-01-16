package ocfl

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestNewObject(t *testing.T) {
	// prepare test
	user := NewUser(`tester`, `tester@nowhere`)
	objectRoot, err := ioutil.TempDir(`./`, `test-object`)
	if err != nil {
		t.Error(err)
	}
	tmpDir, err := ioutil.TempDir(``, `test-object`)
	if err != nil {
		t.Error(err)
	}
	testFile := filepath.Join(tmpDir, `test.txt`)
	if err := ioutil.WriteFile(testFile, []byte(`lol`), FILEMODE); err != nil {
		t.Error(err)
	}
	testDigest, err := Checksum(SHA512, testFile)
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(objectRoot)
	defer os.RemoveAll(tmpDir)

	// create a new object
	object, err := InitObject(objectRoot, `test-object-id`)
	if err != nil {
		t.Error(err)
	}

	// stage for version 1
	stage, err := object.NewStage()
	if err != nil {
		t.Error(err)
	}
	dst := filepath.Join(`dir`, `test-1.txt`)
	if err := stage.Add(testFile, dst); err != nil {
		t.Error(err)
	}
	if err := stage.Rename(dst, `test-2.txt`); err != nil {
		t.Error(err)
	}
	if err := stage.Add(testFile, `test-3.txt`); err != nil {
		t.Error(err)
	}
	if err := stage.Remove(`test-2.txt`); err != nil {
		t.Error(err)
	}
	if err = stage.Commit(user, `commit version 1`); err != nil {
		t.Error(err)
	}
	ePath := filepath.Join(`v1`, `content`, `test-3.txt`)
	if object.inventory.Manifest.GetDigest(Path(ePath)) != Digest(testDigest) {
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
	if err := stage.Remove(`test-3.txt`); err != nil {
		t.Error(err)
	}
	if err = stage.Commit(user, `commit version 2`); err != nil {
		t.Error(err)
	}
	if err = ValidateObject(objectRoot); err != nil {
		t.Error(err)
	}

	// version 3
	stage, err = object.NewStage()
	if err != nil {
		t.Error(err)
	}
	if err := stage.AddRename(testFile, `README.md`); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Errorf(`expected %s to no longer exist`, testFile)
	}
	if err = stage.Commit(user, `commit version 3`); err != nil {
		t.Error(err)
	}
	ePath = filepath.Join(`v3`, `content`, `README.md`)
	if object.inventory.Manifest.GetDigest(Path(ePath)) == `` {
		t.Errorf(`expected %s to exist in manifest`, ePath)
	}
	if err = ValidateObject(objectRoot); err != nil {
		t.Error(err)
	}

}
