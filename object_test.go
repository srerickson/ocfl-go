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
	defer os.RemoveAll(objectRoot)
	object, err := InitObject(objectRoot, `test-object`)
	if err != nil {
		t.Error(err)
	}
	stage, err := object.NewStage()
	if err != nil {
		t.Error(err)
	}
	file, err := stage.OpenFile(LPath(`test.txt`))
	if err != nil {
		t.Error(err)
	}
	defer file.Close()
	_, err = file.WriteString(`testing testing`)
	if err != nil {
		t.Error(err)
	}
	if stage.State[`-`][0] != `test.txt` {
		t.Error(`Expected stage state to include test.txt`)
	}
	if err = stage.Commit(); err != nil {
		t.Error(err)
	}
	if err = ValidateObject(objectRoot); err != nil {
		t.Error(err)
	}
}
