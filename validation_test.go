package ocfl_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/srerickson/ocfl"
)

var fixturePath = filepath.Join(`test`, `fixtures`, `1.0`)
var goodObjPath = filepath.Join(fixturePath, `good-objects`)
var badObjPath = filepath.Join(fixturePath, `bad-objects`)
var warnObjPath = filepath.Join(fixturePath, `warn-objects`)

type objValidationTest struct {
	Path     string
	Expected []*ocfl.OCFLCodeErr
}

var badObjects = []objValidationTest{

	{"E001_extra_dir_in_root", []*ocfl.OCFLCodeErr{&ocfl.ErrE001}},
	{"E001_extra_file_in_root", []*ocfl.OCFLCodeErr{&ocfl.ErrE001}},
	{"E001_v2_file_in_root", []*ocfl.OCFLCodeErr{&ocfl.ErrE001}},
	{"E003_E063_empty", []*ocfl.OCFLCodeErr{&ocfl.ErrE003, &ocfl.ErrE063}},
	{"E003_no_decl", []*ocfl.OCFLCodeErr{&ocfl.ErrE003}},
	{"E007_bad_declaration_contents", []*ocfl.OCFLCodeErr{&ocfl.ErrE007}},
	{"E008_E036_no_versions_no_head", []*ocfl.OCFLCodeErr{&ocfl.ErrE008, &ocfl.ErrE036}},
	{"E015_content_not_in_content_dir", []*ocfl.OCFLCodeErr{&ocfl.ErrE015}},
	{"E023_extra_file", []*ocfl.OCFLCodeErr{&ocfl.ErrE023}},
	{"E023_missing_file", []*ocfl.OCFLCodeErr{&ocfl.ErrE023}},
	{"E036_no_id", []*ocfl.OCFLCodeErr{&ocfl.ErrE036}},
	{"E040_wrong_head_doesnt_exist", []*ocfl.OCFLCodeErr{&ocfl.ErrE040}},
	{"E040_wrong_head_format", []*ocfl.OCFLCodeErr{&ocfl.ErrE040}},
	{"E041_no_manifest", []*ocfl.OCFLCodeErr{&ocfl.ErrE041}},
	{"E049_created_no_timezone", []*ocfl.OCFLCodeErr{&ocfl.ErrE049}},
	{"E049_created_not_to_seconds", []*ocfl.OCFLCodeErr{&ocfl.ErrE049}},
	{"E049_E050_E054_bad_version_block_values", []*ocfl.OCFLCodeErr{&ocfl.ErrE049, &ocfl.ErrE050, &ocfl.ErrE054}},
	{"E050_file_in_manifest_not_used", []*ocfl.OCFLCodeErr{&ocfl.ErrE050}},
	{"E058_no_sidecar", []*ocfl.OCFLCodeErr{&ocfl.ErrE058}},
	{"E063_no_inv", []*ocfl.OCFLCodeErr{&ocfl.ErrE063}},
	{"E064_different_root_and_latest_inventories", []*ocfl.OCFLCodeErr{&ocfl.ErrE064}},
	{"E067_file_in_extensions_dir", []*ocfl.OCFLCodeErr{&ocfl.ErrE067}},
	{"E095_conflicting_logical_paths", []*ocfl.OCFLCodeErr{&ocfl.ErrE095}},
	{"E100_E099_fixity_invalid_content_paths", []*ocfl.OCFLCodeErr{&ocfl.ErrE100, &ocfl.ErrE099}},
	{"E100_E099_manifest_invalid_content_paths", []*ocfl.OCFLCodeErr{&ocfl.ErrE100, &ocfl.ErrE099}},

	// https://github.com/zimeon/ocfl-py/tree/main/extra_fixtures/bad-objects
	// Fixtures referenced below are copyright (c) 2018 Simeon Warner, MIT License
	{"E009_version_two_only", []*ocfl.OCFLCodeErr{&ocfl.ErrE009}},
	{"E012_inconsistent_version_format", []*ocfl.OCFLCodeErr{&ocfl.ErrE012}},
	{"E033_inventory_bad_json", []*ocfl.OCFLCodeErr{&ocfl.ErrE033}},
	{"E046_missing_version_dir", []*ocfl.OCFLCodeErr{&ocfl.ErrE046}},
	{"E050_state_digest_different_case", []*ocfl.OCFLCodeErr{&ocfl.ErrE050}},
	{"E050_state_repeated_digest", []*ocfl.OCFLCodeErr{&ocfl.ErrE050}},
	{"E092_bad_manifest_digest", []*ocfl.OCFLCodeErr{&ocfl.ErrE092}},
	{"E094_message_not_a_string", []*ocfl.OCFLCodeErr{&ocfl.ErrE094}},
	{"E096_manifest_repeated_digest", []*ocfl.OCFLCodeErr{&ocfl.ErrE096}},
	{"E097_fixity_repeated_digest", []*ocfl.OCFLCodeErr{&ocfl.ErrE097}},
	{"E099_bad_content_path_elements", []*ocfl.OCFLCodeErr{&ocfl.ErrE099}},
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
		fsys := os.DirFS(filepath.Join(badObjPath, bad.Path))
		err := ocfl.ValidateObject(fsys)
		if err == nil {
			t.Errorf(`fixture %s: validated but shouldn't`, name)
			continue
		}
		verr, ok := err.(*ocfl.ValidationErr)
		if !ok {
			t.Errorf("fixture %s: expected ocfl.ValidationErr, got: %v", name, err)
			continue
		}
		code := verr.Code()
		var gotExpected bool
		for _, e := range bad.Expected {
			if code == e {
				gotExpected = true
				break
			}
		}
		if !gotExpected {
			t.Errorf(`fixture %s: invalid for the wrong reason. Got %s`, name, code.Code)
		}
	}

	// warning objects
	warnObjects, err := os.ReadDir(warnObjPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range warnObjects {
		p := filepath.Join(warnObjPath, dir.Name())
		err := ocfl.ValidateObject(os.DirFS(p))
		if err != nil {
			t.Errorf(`fixture %s: should be valid, but got error: %v`, dir.Name(), err)
		}
	}

}
