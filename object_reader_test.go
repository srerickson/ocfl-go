package ocfl_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/srerickson/ocfl"
)

var fixturePath = filepath.Join(`test`, `fixtures`, `1.0`)
var goodObjPath = filepath.Join(fixturePath, `good-objects`)
var badObjPath = filepath.Join(fixturePath, `bad-objects`)
var warnObjPath = filepath.Join(fixturePath, `warn-objects`)

func TestObjectReader(t *testing.T) {
	obj, err := ocfl.NewObjectReader(os.DirFS(filepath.Join(goodObjPath, `spec-ex-full`)))
	if err != nil {
		t.Fatal(err)
	}
	err = obj.CUEValudate()
	if err != nil {
		t.Fatal(err)
	}
	v2, err := obj.VersionFS(`v2`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = v2.Open(`foo/bar.xml`)
	if err != nil {
		t.Error(err)
	}
	v3, err := obj.VersionFS(`v3`)
	if err != nil {
		t.Error(err)
	}
	_, err = v3.Open(`not found`)
	if err == nil {
		t.Error(`expected an error`)
	}
}

func TestValidation(t *testing.T) {

	goodObjects, err := os.ReadDir(goodObjPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range goodObjects {
		p := filepath.Join(goodObjPath, dir.Name())
		obj, err := ocfl.NewObjectReader(os.DirFS(p))
		if err != nil {
			t.Fatal(err)
		}
		err = obj.CUEValudate()
		if err != nil {
			t.Error(err)
		}
	}

	badObjects, err := os.ReadDir(badObjPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range badObjects {
		// t.Logf(`testing bad object: %s`, dir.Name())
		p := filepath.Join(badObjPath, dir.Name())
		obj, err := ocfl.NewObjectReader(os.DirFS(p))
		if err != nil {
			// some bad object will fail here - which is OK
			switch dir.Name() {
			case `E003_E034_empty`:
				if errors.Is(err, fs.ErrNotExist) {
					t.Log("caugt EO34")
				}
			case `E034_no_inv`:
				if errors.Is(err, fs.ErrNotExist) {
					t.Log("caught EO34")
				}
			case `E040_wrong_head_format`:
				if _, ok := err.(*json.UnmarshalTypeError); ok {
					if strings.Contains(err.Error(), `Inventory.head`) {
						t.Log(`caugt E040`)
					}
				}
			case `E049_E050_E054_bad_version_block_values`:
				if _, ok := err.(*time.ParseError); ok {
					t.Log("caght E049")
				}
			case `E049_created_no_timezone`:
				if _, ok := err.(*time.ParseError); ok {
					t.Log("caught E049")
				}
			case `E049_created_not_to_seconds`:
				if _, ok := err.(*time.ParseError); ok {
					t.Log("caught E049")
				}
			default:
				t.Fatal(err)
			}
			continue
		}
		err = obj.CUEValudate()
		if err == nil {
			t.Errorf(`bad object did not fail validation as expected: %s`, dir.Name())
		} else {
			t.Logf(`OK: %s`, dir.Name())
		}
	}

}
