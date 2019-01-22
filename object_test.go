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
	objectRoot, err := ioutil.TempDir(`.`, `test-object`)
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(objectRoot)
	tmpDir, err := ioutil.TempDir(`.`, `test-object`)
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(tmpDir)
	testFile := filepath.Join(tmpDir, `test.txt`)
	err = ioutil.WriteFile(testFile, []byte(`lol`), FILEMODE)
	if err != nil {
		t.Error(err)
	}
	testDigest, err := Checksum(SHA512, testFile)
	if err != nil {
		t.Error(err)
	}

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
	err = stage.Add(testFile, dst)
	if err != nil {
		t.Error(err)
	}
	err = stage.Rename(dst, `test-2.txt`)
	if err != nil {
		t.Error(err)
	}
	err = stage.Add(testFile, `test-3.txt`)
	if err != nil {
		t.Error(err)
	}
	err = stage.Remove(`test-2.txt`)
	if err != nil {
		t.Error(err)
	}
	err = stage.Commit(user, `commit version 1`)
	if err != nil {
		t.Error(err)
	}
	ePath := filepath.Join(`v1`, `content`, `test-3.txt`)
	if object.inventory.Manifest.GetDigest(ePath) != testDigest {
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
	err = stage.Remove(`test-3.txt`)
	if err != nil {
		t.Error(err)
	}
	err = stage.Commit(user, `commit version 2`)
	if err != nil {
		t.Error(err)
	}
	err = ValidateObject(objectRoot)
	if err != nil {
		t.Error(err)
	}
}

func TestGetObject(t *testing.T) {
	o, err := GetObject(`nothing`)
	if err == nil {
		t.Error(`expected an error`)
	}
	path := filepath.Join(`test`, `fixtures`, `1.0`, `bad-objects`, `bad03_no_inv`)
	o, err = GetObject(path)
	if err == nil {
		t.Error(`expected an error`)
	}
	path = filepath.Join(`test`, `fixtures`, `1.0`, `objects`, `spec-ex-full`)
	o, err = GetObject(path)
	if err != nil {
		t.Error(err)
	}
	_, err = o.Open(filepath.Join(`foo`, `bar.xml`))
	if err != nil {
		t.Error(err)
	}
}

func TestObjectIterate(t *testing.T) {
	path := filepath.Join(`test`, `fixtures`, `1.0`, `objects`, `spec-ex-full`)
	o, err := GetObject(path)
	if err != nil {
		t.Error(err)
	}
	var gotFiles bool
	files, err := o.Iterate()
	if err != nil {
		t.Error(err)
	}
	for range files {
		gotFiles = true
	}
	if !gotFiles {
		t.Error(`expected to get files`)
	}
}

func TestChangelessCommit(t *testing.T) {
	path := filepath.Join(`test`, `fixtures`, `1.0`, `objects`, `spec-ex-full`)
	o, err := GetObject(path)
	if err != nil {
		t.Error(err)
	}
	stage, _ := o.NewStage()
	err = stage.Commit(NewUser(``, ``), `message`)
	if err == nil {
		t.Error(`expected an error`)
	}
}
