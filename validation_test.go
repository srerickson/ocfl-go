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

type objValidationTest struct {
	Path     string
	Expected []*ocfl.OCFLCodeErr
}

var badObjects = []objValidationTest{
	{filepath.Join(badObjPath, "E001_extra_dir_in_root"), []*ocfl.OCFLCodeErr{&ocfl.ErrE001}},
	{filepath.Join(badObjPath, "E001_extra_file_in_root"), []*ocfl.OCFLCodeErr{&ocfl.ErrE001}},
	{filepath.Join(badObjPath, "E003_E034_empty"), []*ocfl.OCFLCodeErr{&ocfl.ErrE003, &ocfl.ErrE034}},
	{filepath.Join(badObjPath, "E003_no_decl"), []*ocfl.OCFLCodeErr{&ocfl.ErrE003}},
	{filepath.Join(badObjPath, "E007_bad_declaration_contents"), []*ocfl.OCFLCodeErr{&ocfl.ErrE007}},
	{filepath.Join(badObjPath, "E008_E036_no_versions_no_head"), []*ocfl.OCFLCodeErr{&ocfl.ErrE008, &ocfl.ErrE036}},
	{filepath.Join(badObjPath, "E015_content_not_in_content_dir"), []*ocfl.OCFLCodeErr{&ocfl.ErrE015}},
	{filepath.Join(badObjPath, "E023_extra_file"), []*ocfl.OCFLCodeErr{&ocfl.ErrE023}},
	{filepath.Join(badObjPath, "E023_missing_file"), []*ocfl.OCFLCodeErr{&ocfl.ErrE023}},
	{filepath.Join(badObjPath, "E034_no_inv"), []*ocfl.OCFLCodeErr{&ocfl.ErrE034}},
	{filepath.Join(badObjPath, "E036_no_id"), []*ocfl.OCFLCodeErr{&ocfl.ErrE036}},
	{filepath.Join(badObjPath, "E040_wrong_head_doesnt_exist"), []*ocfl.OCFLCodeErr{&ocfl.ErrE040}},
	{filepath.Join(badObjPath, "E040_wrong_head_format"), []*ocfl.OCFLCodeErr{&ocfl.ErrE040}},
	{filepath.Join(badObjPath, "E041_no_manifest"), []*ocfl.OCFLCodeErr{&ocfl.ErrE041}},
	{filepath.Join(badObjPath, "E049_created_no_timezone"), []*ocfl.OCFLCodeErr{&ocfl.ErrE049}},
	{filepath.Join(badObjPath, "E049_created_not_to_seconds"), []*ocfl.OCFLCodeErr{&ocfl.ErrE049}},
	{filepath.Join(badObjPath, "E049_E050_E054_bad_version_block_values"), []*ocfl.OCFLCodeErr{&ocfl.ErrE049, &ocfl.ErrE050, &ocfl.ErrE054}},
	{filepath.Join(badObjPath, "E050_file_in_manifest_not_used"), []*ocfl.OCFLCodeErr{&ocfl.ErrE050}},
	{filepath.Join(badObjPath, "E058_no_sidecar"), []*ocfl.OCFLCodeErr{&ocfl.ErrE058}},
	{filepath.Join(badObjPath, "E064_different_root_and_latest_inventories"), []*ocfl.OCFLCodeErr{&ocfl.ErrE064}},
	{filepath.Join(badObjPath, "E067_file_in_extensions_dir"), []*ocfl.OCFLCodeErr{&ocfl.ErrE067}},
	{filepath.Join(badObjPath, "E095_conflicting_logical_paths"), []*ocfl.OCFLCodeErr{&ocfl.ErrE095}},

	// https://github.com/zimeon/ocfl-py/tree/main/extra_fixtures/bad-objects
	// Fixtures referenced below are copyright (c) 2018 Simeon Warner, MIT License
	{filepath.Join(badObjPath, "E009_version_two_only"), []*ocfl.OCFLCodeErr{&ocfl.ErrE009}},
	{filepath.Join(badObjPath, "E012_inconsistent_version_format"), []*ocfl.OCFLCodeErr{&ocfl.ErrE012}},
	{filepath.Join(badObjPath, "E033_inventory_bad_json"), []*ocfl.OCFLCodeErr{&ocfl.ErrE033}},
	{filepath.Join(badObjPath, "E046_missing_version_dir"), []*ocfl.OCFLCodeErr{&ocfl.ErrE046}},
	{filepath.Join(badObjPath, "E050_state_digest_different_case"), []*ocfl.OCFLCodeErr{&ocfl.ErrE050}},
	{filepath.Join(badObjPath, "E050_state_repeated_digest"), []*ocfl.OCFLCodeErr{&ocfl.ErrE050}},
	{filepath.Join(badObjPath, "E092_bad_manifest_digest"), []*ocfl.OCFLCodeErr{&ocfl.ErrE092}},
	{filepath.Join(badObjPath, "E094_message_not_a_string"), []*ocfl.OCFLCodeErr{&ocfl.ErrE094}},
	{filepath.Join(badObjPath, "E096_manifest_repeated_digest"), []*ocfl.OCFLCodeErr{&ocfl.ErrE096}},
	{filepath.Join(badObjPath, "E097_fixity_repeated_digest"), []*ocfl.OCFLCodeErr{&ocfl.ErrE097}},
	{filepath.Join(badObjPath, "E099_bad_content_path_elements"), []*ocfl.OCFLCodeErr{&ocfl.ErrE099}},
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
}
