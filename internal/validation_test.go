package internal_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/srerickson/ocfl/internal"
)

var codeRegexp = regexp.MustCompile(`^E\d{3}$`)
var fixturePath = filepath.Join(`..`, `test`, `fixtures`, `1.0`)
var goodObjPath = filepath.Join(fixturePath, `good-objects`)
var badObjPath = filepath.Join(fixturePath, `bad-objects`)
var warnObjPath = filepath.Join(fixturePath, `warn-objects`)

func TestValidation(t *testing.T) {

	goodObjects, err := os.ReadDir(goodObjPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range goodObjects {
		p := filepath.Join(goodObjPath, dir.Name())
		result := internal.ValidateObject(os.DirFS(p))
		if !result.Valid() {
			t.Errorf(`fixture %s: should be valid, but got errors:`, dir.Name())
			for _, err := range result.Fatal() {
				t.Errorf(`--> %s`, err.Error())
			}
		}
	}
	badObjects, err := os.ReadDir(badObjPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range badObjects {
		if !dir.IsDir() {
			continue
		}
		name := dir.Name()
		expectedCode := []string{}
		for _, part := range strings.Split(name, "_") {
			if codeRegexp.MatchString(part) {
				expectedCode = append(expectedCode, part)
			}
		}
		fsys := os.DirFS(filepath.Join(badObjPath, name))
		result := internal.ValidateObject(fsys)
		if result.Valid() {
			t.Errorf(`fixture %s: validated but shouldn't`, name)
			continue
		}
		for _, err := range result.Fatal() {
			var gotExpected bool
			for _, e := range expectedCode {
				if err.Code() == e {
					gotExpected = true
					break
				}
			}
			if !gotExpected {
				t.Errorf(`fixture %s: invalid for the wrong reason. Got %s`, name, err.Error())
			}
		}
	}

	// warning objects
	warnObjects, err := os.ReadDir(warnObjPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range warnObjects {
		p := filepath.Join(warnObjPath, dir.Name())
		result := internal.ValidateObject(os.DirFS(p))
		if !result.Valid() {
			t.Errorf(`fixture %s: should be valid, but got errors:`, dir.Name())
			for _, err := range result.Fatal() {
				t.Errorf(`--> %s`, err.Error())
			}
		}
	}

}
