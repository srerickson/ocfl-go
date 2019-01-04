package namaste

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestSearch(t *testing.T) {
	fixtures := filepath.FromSlash(`../test/fixtures`)
	results, err := SearchTypePattern(fixtures, `ocfl_object.*`)
	if err != nil {
		t.Error(err)
	}
	if len(results) != 7 {
		t.Error(`expected 6 results from SearchTypePattern`)
	}
}

func TestSetType(t *testing.T) {
	tvalue := `ocfl_object_1.0`
	fvalue := "ocfl object 1.0\n"
	tmp, err := ioutil.TempDir(``, `ocfl-test-`)
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(tmp)
	SetType(tmp, tvalue, fvalue)
	f, err := ioutil.ReadFile(filepath.Join(tmp, `0=`+tvalue))
	if err != nil {
		t.Error(err)
	}
	if string(f) != fvalue {
		t.Error(`SetType failed: fvalue read does not match fvalue set`)
	}
	if match, err := MatchTypePattern(tmp, tvalue); !match {
		t.Error(err)
	}

	if match, _ := MatchTypePattern(tmp, `none`); match {
		t.Error(`MatchTypePattern should not have matched`)
	}
}
