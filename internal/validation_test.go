package internal_test

import (
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
	Expected []string
}

var badObjects = []objValidationTest{

	{"E001_extra_dir_in_root", []string{"E001"}},
	{"E001_extra_file_in_root", []string{"E001"}},
	{"E001_v2_file_in_root", []string{"E001"}},
	{"E003_E063_empty", []string{"E003", "E063"}},
	{"E003_no_decl", []string{"E003"}},
	{"E007_bad_declaration_contents", []string{"E007"}},
	{"E008_E036_no_versions_no_head", []string{"E008", "E036"}},
	{"E015_content_not_in_content_dir", []string{"E015"}},
	{"E023_extra_file", []string{"E023"}},
	{"E023_missing_file", []string{"E023"}},
	{"E036_no_id", []string{"E036"}},
	{"E040_wrong_head_doesnt_exist", []string{"E040"}},
	{"E040_wrong_head_format", []string{"E040"}},
	{"E041_no_manifest", []string{"E041"}},
	{"E049_created_no_timezone", []string{"E049"}},
	{"E049_created_not_to_seconds", []string{"E049"}},
	{"E049_E050_E054_bad_version_block_values", []string{"E049", "E050", "E054"}},
	{"E050_file_in_manifest_not_used", []string{"E050"}},
	{"E058_no_sidecar", []string{"E058"}},
	{"E063_no_inv", []string{"E063"}},
	{"E064_different_root_and_latest_inventories", []string{"E064"}},
	{"E067_file_in_extensions_dir", []string{"E067"}},
	{"E095_conflicting_logical_paths", []string{"E095"}},
	{"E100_E099_fixity_invalid_content_paths", []string{"E100", "E099"}},
	{"E100_E099_manifest_invalid_content_paths", []string{"E100", "E099"}},

	// https://github.com/zimeon/ocfl-py/tree/main/extra_fixtures/bad-objects
	// Fixtures referenced below are copyright (c) 2018 Simeon Warner, MIT License
	{"E009_version_two_only", []string{"E009"}},
	{"E012_inconsistent_version_format", []string{"E012"}},
	{"E033_inventory_bad_json", []string{"E033"}},
	{"E046_missing_version_dir", []string{"E046"}},
	{"E050_state_digest_different_case", []string{"E050"}},
	{"E050_state_repeated_digest", []string{"E050"}},
	{"E092_bad_manifest_digest", []string{"E092"}},
	{"E094_message_not_a_string", []string{"E094"}},
	{"E096_manifest_repeated_digest", []string{"E096"}},
	{"E097_fixity_repeated_digest", []string{"E097"}},
	{"E099_bad_content_path_elements", []string{"E099"}},
}

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
	for _, bad := range badObjects {
		name := filepath.Base(bad.Path)
		fsys := os.DirFS(filepath.Join(badObjPath, bad.Path))
		result := internal.ValidateObject(fsys)
		if result.Valid() {
			t.Errorf(`fixture %s: validated but shouldn't`, name)
			continue
		}
		for _, err := range result.Fatal() {
			var gotExpected bool
			for _, e := range bad.Expected {
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
