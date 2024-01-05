package ocfl_test

import (
	"reflect"
	"testing"

	"github.com/srerickson/ocfl-go"
)

var invalidPaths = []string{
	"",
	".",
	"/file1.txt",
	"../file1.txt",
	"./file.txt",
	"dir//file.txt",
	"dir/./file.txt",
	"dir/../file.txt",
}

var validMaps = map[string]ocfl.DigestMap{
	"empty":       {},
	"single file": {"abcde": {"file.txt"}},
	"multiple files": {
		"abcde1": {"file.txt", "file2.txt"},
		"abcde2": {"nested/directory/file.csv"},
	},
}

var invalidMaps = map[string]ocfl.DigestMap{
	"missing paths": {
		"abcd": {},
	},
	"duplicate path for same digest": {
		"abcd": {"file.txt", "file.txt"},
	},
	"duplicate path for separate digests": {
		"abcd1": {"file.txt"},
		"abcd2": {"file.txt"},
	},
	"directory/file conflict": {
		"abcd1": {"a/b"},
		"abcd2": {"a/b/file.txt"},
	},
	"duplicate digests, differenct cases": {
		"abcd1": {"file1.txt"},
		"ABCD1": {"file2.txt"},
	},
}

func testMapValid(t *testing.T, desc string, digests ocfl.DigestMap, expOK bool) {
	t.Helper()
	t.Run(desc, func(t *testing.T) {
		err := digests.Valid()
		if err == nil && !expOK {
			t.Fatal("invalid map was found to be valid")
		}
		if err != nil && expOK {
			t.Fatalf("valid map was found to be invalid, with error: %s", err)
		}
	})

}

func TestDigestMapValid(t *testing.T) {
	for _, p := range invalidPaths {
		desc := "invalid path: " + p
		digest := ocfl.DigestMap{"abcd": {p}}
		testMapValid(t, desc, digest, false)
	}
	for desc, digests := range invalidMaps {
		testMapValid(t, desc, digests, false)
	}
	for desc, digests := range validMaps {
		testMapValid(t, desc, digests, true)
	}
}

func TestMapEq(t *testing.T) {
	type eqTest struct {
		a      ocfl.DigestMap
		b      ocfl.DigestMap
		expect bool
	}
	eqTests := map[string]eqTest{
		"empty maps": {expect: true},
		"same": {
			a:      ocfl.DigestMap{"abc": {"1", "2", "3"}},
			b:      ocfl.DigestMap{"abc": {"1", "2", "3"}},
			expect: true},
		"same with mixed case dgiests": {
			a:      ocfl.DigestMap{"ABC": {"1", "2", "3"}},
			b:      ocfl.DigestMap{"abc": {"1", "2", "3"}},
			expect: true},
		"same with different ordered paths": {
			a:      ocfl.DigestMap{"abc": {"1", "2", "3"}},
			b:      ocfl.DigestMap{"abc": {"1", "3", "2"}},
			expect: true},
		"different digests": {
			a:      ocfl.DigestMap{"abc1": {"1", "2", "3"}},
			b:      ocfl.DigestMap{"abc2": {"1", "2", "3"}},
			expect: false},
		"different paths": {
			a:      ocfl.DigestMap{"abc": {"1", "2"}},
			b:      ocfl.DigestMap{"abc": {"1", "2", "3"}},
			expect: false},
	}
	for n, eqt := range eqTests {
		t.Run(n, func(t *testing.T) {
			if eq := eqt.a.Eq(eqt.b); eqt.expect != eq {
				t.Errorf("Eq() got=%v, expect=%v", eq, eqt.expect)
			}
		})
	}
}

func TestDigestMapMerge(t *testing.T) {
	type mergeTest struct {
		m1          ocfl.DigestMap
		m2          ocfl.DigestMap
		replace     bool
		resultPaths ocfl.PathMap
		isValid     bool
	}
	mergeTests := map[string]mergeTest{
		"valid-empty": {
			m1:          ocfl.DigestMap{},
			m2:          ocfl.DigestMap{},
			replace:     false,
			resultPaths: ocfl.PathMap{},
			isValid:     true,
		},
		"valid-m1-empty": {
			m1:          ocfl.DigestMap{},
			m2:          ocfl.DigestMap{"abc1": {"dir/file1"}},
			replace:     false,
			resultPaths: ocfl.PathMap{"dir/file1": "abc1"},
			isValid:     true,
		},
		"valid-m2-empty": {
			m1:          ocfl.DigestMap{"abc1": {"dir/file1"}},
			m2:          ocfl.DigestMap{},
			replace:     false,
			resultPaths: ocfl.PathMap{"dir/file1": "abc1"},
			isValid:     true,
		},
		"valid-mixed-digest": {
			m1: ocfl.DigestMap{
				"ABC1": {"dir/file1"},
				"ABC2": {"dir/file2"},
			},
			m2:      ocfl.DigestMap{"abc1": {"dir/file1"}},
			replace: false,
			resultPaths: ocfl.PathMap{
				"dir/file1": "abc1",
				"dir/file2": "abc2",
			},
			isValid: true,
		},
		"valid-combine-digest": {
			m1:      ocfl.DigestMap{"abc1": {"dir/file1"}},
			m2:      ocfl.DigestMap{"abc1": {"dir/file2"}},
			replace: false,
			resultPaths: ocfl.PathMap{
				"dir/file1": "abc1",
				"dir/file2": "abc1",
			},
			isValid: true,
		},
		"invalid-noreplace": {
			m1:          ocfl.DigestMap{"abc1": {"dir/file"}},
			m2:          ocfl.DigestMap{"abc2": {"dir/file"}},
			replace:     false,
			resultPaths: nil,
			isValid:     false,
		},
		"valid-replace": {
			m1:          ocfl.DigestMap{"abc1": {"dir/file"}},
			m2:          ocfl.DigestMap{"abc2": {"dir/file"}},
			replace:     true,
			resultPaths: ocfl.PathMap{"dir/file": "abc2"},
			isValid:     true,
		},
		"invalid-conflict_existing_file": {
			m1:          ocfl.DigestMap{"abc1": {"dir/file"}},
			m2:          ocfl.DigestMap{"abc2": {"dir/file/file"}},
			replace:     true,
			resultPaths: nil,
			isValid:     false,
		},
		"invalid-conflict_existing_dir": {
			m1:          ocfl.DigestMap{"abc1": {"dir/file"}},
			m2:          ocfl.DigestMap{"abc2": {"dir"}},
			replace:     true,
			resultPaths: nil,
			isValid:     false,
		},
	}
	for name, mtest := range mergeTests {
		t.Run(name, func(t *testing.T) {
			result, err := mtest.m1.Merge(mtest.m2, mtest.replace)
			if err != nil && mtest.isValid {
				t.Error("Merge() returned error for valid case: ", err)
			}
			if err == nil && !mtest.isValid {
				t.Error("Merge() returned no error for invalid case")
			}
			if mtest.isValid {
				resultPaths := result.PathMap()
				if !reflect.DeepEqual(resultPaths, mtest.resultPaths) {
					t.Errorf("Merge() return unexpcted result.\n got=%v\n expected=%v", resultPaths, mtest.resultPaths)
				}
			}
		})
	}
}

func TestDigestMapRemap(t *testing.T) {
	t.Run("Remove", func(t *testing.T) {
		// Remove on its own
		out := ocfl.Remove("delete.txt")("abc", []string{"delete.txt", "keep.txt"})
		if len(out) != 1 || out[0] != "keep.txt" {
			t.Error("Remove() didn't remove the expected file")
		}
		// Remap with Remove
		dm := ocfl.DigestMap{
			"abc1": {"keep.txt", "delete.txt"},
			"abc2": {"save.txt"},
		}
		result, err := dm.Remap(ocfl.Remove("delete.txt"))
		if err != nil {
			t.Fatal("Remap() with Remove() unexpected error:", err)
		}
		if result.GetDigest("delete.txt") != "" {
			t.Error("Remap() with Remove() file not removed")
		}
		if result.GetDigest("keep.txt") == "" || result.GetDigest("save.txt") == "" {
			t.Error("Remap() with Remove() removed a file it shouldn't have")
		}
	})
}
