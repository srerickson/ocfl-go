package ocfl_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/srerickson/ocfl"
)

var fixturePath = filepath.Join(`test`, `fixtures`, `1.0`)
var goodObjPath = filepath.Join(fixturePath, `good-objects`)
var badObjPath = filepath.Join(fixturePath, `bad-objects`)

type objValidationTest struct {
	Path     string
	Expected []error
}

var badObjects = []objValidationTest{
	{filepath.Join(badObjPath, "E001_extra_dir_in_root"), []error{&ocfl.ErrE001}},
	{filepath.Join(badObjPath, "E001_extra_file_in_root"), []error{&ocfl.ErrE001}},
	{filepath.Join(badObjPath, "E003_E034_empty"), []error{&ocfl.ErrE003, &ocfl.ErrE034}},
	{filepath.Join(badObjPath, "E003_no_decl"), []error{&ocfl.ErrE003}},
	{filepath.Join(badObjPath, "E007_bad_declaration_contents"), []error{&ocfl.ErrE007}},
	{filepath.Join(badObjPath, "E008_E036_no_versions_no_head"), []error{&ocfl.ErrE008, &ocfl.ErrE036}},
	{filepath.Join(badObjPath, "E015_content_not_in_content_dir"), []error{&ocfl.ErrE015}},
	{filepath.Join(badObjPath, "E023_extra_file"), []error{&ocfl.ErrE023}},
	{filepath.Join(badObjPath, "E023_missing_file"), []error{&ocfl.ErrE023}},
	{filepath.Join(badObjPath, "E034_no_inv"), []error{&ocfl.ErrE034}},
	{filepath.Join(badObjPath, "E036_no_id"), []error{&ocfl.ErrE036}},
	{filepath.Join(badObjPath, "E040_wrong_head_doesnt_exist"), []error{&ocfl.ErrE040}},
	{filepath.Join(badObjPath, "E040_wrong_head_format"), []error{&ocfl.ErrE040}},
	{filepath.Join(badObjPath, "E041_no_manifest"), []error{&ocfl.ErrE041}},
	{filepath.Join(badObjPath, "E049_created_no_timezone"), []error{&ocfl.ErrE049}},
	{filepath.Join(badObjPath, "E049_created_not_to_seconds"), []error{&ocfl.ErrE049}},
	{filepath.Join(badObjPath, "E049_E050_E054_bad_version_block_values"), []error{&ocfl.ErrE049, &ocfl.ErrE050, &ocfl.ErrE054}},
	{filepath.Join(badObjPath, "E050_file_in_manifest_not_used"), []error{&ocfl.ErrE050}},
	{filepath.Join(badObjPath, "E058_no_sidecar"), []error{&ocfl.ErrE058}},
	{filepath.Join(badObjPath, "E064_different_root_and_latest_inventories"), []error{&ocfl.ErrE064}},
	{filepath.Join(badObjPath, "E067_file_in_extensions_dir"), []error{&ocfl.ErrE067}},
	{filepath.Join(badObjPath, "E095_conflicting_logical_paths"), []error{&ocfl.ErrE095}},
}

func TestValidation(t *testing.T) {

	goodObjects, err := os.ReadDir(goodObjPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range goodObjects {
		p := filepath.Join(goodObjPath, dir.Name())
		err := ocfl.ValidateObject(os.DirFS(p))
		if err != nil {
			t.Errorf(`fixture %s: should be valid, but got error: %v`, dir.Name(), err)
		}
	}

	for _, bad := range badObjects {
		name := filepath.Base(bad.Path)
		err := ocfl.ValidateObject(os.DirFS(bad.Path))
		if err == nil {
			t.Errorf(`fixture %s: validated but shouldn't`, name)
			continue
		}
		var gotExpected bool
		for _, e := range bad.Expected {
			if errors.Is(err, e) {
				gotExpected = true
				break
			}
		}
		if !gotExpected {
			t.Errorf(`fixture %s: invalid for the wrong reason: %v`, name, err)
		}

	}

}
