package ocfl

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestNewObject(t *testing.T) {
	objectRoot, err := ioutil.TempDir(`./`, `test-object`)
	if err != nil {
		t.Error(err)
	}
	//defer os.RemoveAll(objectRoot)
	object, err := InitObject(objectRoot, `test-object`)
	if err != nil {
		t.Error(err)
	}
	stage, err := object.NewStage()
	if err != nil {
		t.Error(err)
	}
	file, err := stage.OpenFile(`test.txt`, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		t.Error(err)
	}
	_, err = file.WriteString(`testing testing`)
	if err != nil {
		t.Error(err)
	}
	file.Close()
	stage.Rename(`test.txt`, `test2.txt`)
	if err = stage.Commit(); err != nil {
		t.Error(err)
	}
	if err = ValidateObject(objectRoot); err != nil {
		t.Error(err)
	}
}
