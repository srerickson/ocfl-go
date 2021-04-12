package version_fs

import (
	"encoding/json"
	"os"
	"testing"
)

func TestAdd(t *testing.T) {
	d := DirEntries{}
	err := d.add("a/b/c/d/r/asdf.txt", "dbbnjs")
	if err != nil {
		t.Error(err)
	}
	err = d.add("a/b/c/d/e/f.txt", "anoth")
	if err != nil {
		t.Error(err)
	}
	err = d.add("a/b/c/d/e/f.txt/explode", "another")
	if err != nil {
		t.Error(err)
	}
	json.NewEncoder(os.Stdout).Encode(d)
}
