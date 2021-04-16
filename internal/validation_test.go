package internal_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/srerickson/ocfl/internal"
)

var fixturePath = filepath.Join(`..`, `test`, `fixtures`, `1.0`)
var goodObjPath = filepath.Join(fixturePath, `good-objects`)
var badObjPath = filepath.Join(fixturePath, `bad-objects`)
var warnObjPath = filepath.Join(fixturePath, `warn-objects`)

type objValidationTest struct {
	Path     string
	Expected []*internal.OCFLCodeErr
}

var badObjects = []objValidationTest{

	{"E001_extra_dir_in_root", []*internal.OCFLCodeErr{&internal.ErrE001}},
	{"E001_extra_file_in_root", []*internal.OCFLCodeErr{&internal.ErrE001}},
	{"E001_v2_file_in_root", []*internal.OCFLCodeErr{&internal.ErrE001}},
	{"E003_E063_empty", []*internal.OCFLCodeErr{&internal.ErrE003, &internal.ErrE063}},
	{"E003_no_decl", []*internal.OCFLCodeErr{&internal.ErrE003}},
	{"E007_bad_declaration_contents", []*internal.OCFLCodeErr{&internal.ErrE007}},
	{"E008_E036_no_versions_no_head", []*internal.OCFLCodeErr{&internal.ErrE008, &internal.ErrE036}},
	{"E015_content_not_in_content_dir", []*internal.OCFLCodeErr{&internal.ErrE015}},
	{"E023_extra_file", []*internal.OCFLCodeErr{&internal.ErrE023}},
	{"E023_missing_file", []*internal.OCFLCodeErr{&internal.ErrE023}},
	{"E036_no_id", []*internal.OCFLCodeErr{&internal.ErrE036}},
	{"E040_wrong_head_doesnt_exist", []*internal.OCFLCodeErr{&internal.ErrE040}},
	{"E040_wrong_head_format", []*internal.OCFLCodeErr{&internal.ErrE040}},
	{"E041_no_manifest", []*internal.OCFLCodeErr{&internal.ErrE041}},
	{"E049_created_no_timezone", []*internal.OCFLCodeErr{&internal.ErrE049}},
	{"E049_created_not_to_seconds", []*internal.OCFLCodeErr{&internal.ErrE049}},
	{"E049_E050_E054_bad_version_block_values", []*internal.OCFLCodeErr{&internal.ErrE049, &internal.ErrE050, &internal.ErrE054}},
	{"E050_file_in_manifest_not_used", []*internal.OCFLCodeErr{&internal.ErrE050}},
	{"E058_no_sidecar", []*internal.OCFLCodeErr{&internal.ErrE058}},
	{"E063_no_inv", []*internal.OCFLCodeErr{&internal.ErrE063}},
	{"E064_different_root_and_latest_inventories", []*internal.OCFLCodeErr{&internal.ErrE064}},
	{"E067_file_in_extensions_dir", []*internal.OCFLCodeErr{&internal.ErrE067}},
	{"E095_conflicting_logical_paths", []*internal.OCFLCodeErr{&internal.ErrE095}},
	{"E100_E099_fixity_invalid_content_paths", []*internal.OCFLCodeErr{&internal.ErrE100, &internal.ErrE099}},
	{"E100_E099_manifest_invalid_content_paths", []*internal.OCFLCodeErr{&internal.ErrE100, &internal.ErrE099}},

	// https://github.com/zimeon/ocfl-py/tree/main/extra_fixtures/bad-objects
	// Fixtures referenced below are copyright (c) 2018 Simeon Warner, MIT License
	{"E009_version_two_only", []*internal.OCFLCodeErr{&internal.ErrE009}},
	{"E012_inconsistent_version_format", []*internal.OCFLCodeErr{&internal.ErrE012}},
	{"E033_inventory_bad_json", []*internal.OCFLCodeErr{&internal.ErrE033}},
	{"E046_missing_version_dir", []*internal.OCFLCodeErr{&internal.ErrE046}},
	{"E050_state_digest_different_case", []*internal.OCFLCodeErr{&internal.ErrE050}},
	{"E050_state_repeated_digest", []*internal.OCFLCodeErr{&internal.ErrE050}},
	{"E092_bad_manifest_digest", []*internal.OCFLCodeErr{&internal.ErrE092}},
	{"E094_message_not_a_string", []*internal.OCFLCodeErr{&internal.ErrE094}},
	{"E096_manifest_repeated_digest", []*internal.OCFLCodeErr{&internal.ErrE096}},
	{"E097_fixity_repeated_digest", []*internal.OCFLCodeErr{&internal.ErrE097}},
	{"E099_bad_content_path_elements", []*internal.OCFLCodeErr{&internal.ErrE099}},
}

func TestValidation(t *testing.T) {

	goodObjects, err := os.ReadDir(goodObjPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range goodObjects {
		p := filepath.Join(goodObjPath, dir.Name())
		err := internal.ValidateObject(os.DirFS(p))
		if err != nil {
			t.Errorf(`fixture %s: should be valid, but got error: %v`, dir.Name(), err)
		}
	}
	for _, bad := range badObjects {
		name := filepath.Base(bad.Path)
		fsys := os.DirFS(filepath.Join(badObjPath, bad.Path))
		result := internal.ValidateObject(fsys)
		if result == nil {
			t.Errorf(`fixture %s: validated but shouldn't`, name)
			continue
		}
		for _, err := range result.Fatal {
			var verr *internal.ValidationErr
			if !errors.As(err, &verr) {
				t.Errorf("fixture %s: expected internal.ValidationErr, got: %v", name, err)
				continue
			}
			var gotExpected bool
			for _, e := range bad.Expected {
				if verr.Code() == e {
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
		err := internal.ValidateObject(os.DirFS(p))
		if err != nil {
			t.Errorf(`fixture %s: should be valid, but got error: %v`, dir.Name(), err)
		}
	}

}
