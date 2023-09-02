package ocfl_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/srerickson/ocfl"
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

var validMaps = map[string]map[string][]string{
	"empty":       {},
	"single file": {"abcde": {"file.txt"}},
	"multiple files": {
		"abcde1": {"file.txt", "file2.txt"},
		"abcde2": {"nested/directory/file.csv"},
	},
}

var invalidMaps = map[string]map[string][]string{
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

func testMapValid(t *testing.T, desc string, digests map[string][]string, expOK bool) {
	t.Helper()
	t.Run(desc, func(t *testing.T) {
		_, err := ocfl.NewDigestMap(digests)
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
		digest := map[string][]string{"abcd": {p}}
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
		a      map[string][]string
		b      map[string][]string
		expect bool
	}
	eqTests := map[string]eqTest{
		"empty maps": {expect: true},
		"same": {
			a:      map[string][]string{"abc": {"1", "2", "3"}},
			b:      map[string][]string{"abc": {"1", "2", "3"}},
			expect: true},
		"same with mixed case dgiests": {
			a:      map[string][]string{"ABC": {"1", "2", "3"}},
			b:      map[string][]string{"abc": {"1", "2", "3"}},
			expect: true},
		"same with different ordered paths": {
			a:      map[string][]string{"abc": {"1", "2", "3"}},
			b:      map[string][]string{"abc": {"1", "3", "2"}},
			expect: true},
		"different digests": {
			a:      map[string][]string{"abc1": {"1", "2", "3"}},
			b:      map[string][]string{"abc2": {"1", "2", "3"}},
			expect: false},
		"different paths": {
			a:      map[string][]string{"abc": {"1", "2"}},
			b:      map[string][]string{"abc": {"1", "2", "3"}},
			expect: false},
	}
	for n, eqt := range eqTests {
		t.Run(n, func(t *testing.T) {
			m1, _ := ocfl.NewDigestMap(eqt.a)
			m2, _ := ocfl.NewDigestMap(eqt.b)
			if eq := m1.Eq(m2); eqt.expect != eq {
				t.Errorf("Eq() got=%v, expect=%v", eq, eqt.expect)
			}
		})
	}
}

func TestDigestMapMerge(t *testing.T) {
	type mergeTest struct {
		m1          map[string][]string
		m2          map[string][]string
		replace     bool
		resultPaths map[string]string
		isValid     bool
	}
	mergeTests := map[string]mergeTest{
		"valid-empty": {
			m1:          map[string][]string{},
			m2:          map[string][]string{},
			replace:     false,
			resultPaths: map[string]string{},
			isValid:     true,
		},
		"valid-m1-empty": {
			m1:      map[string][]string{},
			m2:      map[string][]string{"abc1": {"dir/file1"}},
			replace: false,
			resultPaths: map[string]string{
				"dir/file1": "abc1",
			},
			isValid: true,
		},
		"valid-m2-empty": {
			m1:      map[string][]string{"abc1": {"dir/file1"}},
			m2:      map[string][]string{},
			replace: false,
			resultPaths: map[string]string{
				"dir/file1": "abc1",
			},
			isValid: true,
		},
		"valid-mixed-digest": {
			m1: map[string][]string{
				"ABC1": {"dir/file1"},
				"ABC2": {"dir/file2"},
			},
			m2:      map[string][]string{"abc1": {"dir/file1"}},
			replace: false,
			resultPaths: map[string]string{
				"dir/file1": "abc1",
				"dir/file2": "abc2",
			},
			isValid: true,
		},
		"valid-combine-digest": {
			m1:      map[string][]string{"abc1": {"dir/file1"}},
			m2:      map[string][]string{"abc1": {"dir/file2"}},
			replace: false,
			resultPaths: map[string]string{
				"dir/file1": "abc1",
				"dir/file2": "abc1",
			},
			isValid: true,
		},
		"valid-keep": {
			m1:          map[string][]string{"abc1": {"dir/file"}},
			m2:          map[string][]string{"abc2": {"dir/file"}},
			replace:     false,
			resultPaths: map[string]string{"dir/file": "abc1"},
			isValid:     true,
		},
		"valid-replace": {
			m1:          map[string][]string{"abc1": {"dir/file"}},
			m2:          map[string][]string{"abc2": {"dir/file"}},
			replace:     true,
			resultPaths: map[string]string{"dir/file": "abc2"},
			isValid:     true,
		},
		"invalid-conflict_existing_file": {
			m1:          map[string][]string{"abc1": {"dir/file"}},
			m2:          map[string][]string{"abc2": {"dir/file/file"}},
			replace:     true,
			resultPaths: nil,
			isValid:     false,
		},
		"invalid-conflict_existing_dir": {
			m1:          map[string][]string{"abc1": {"dir/file"}},
			m2:          map[string][]string{"abc2": {"dir"}},
			replace:     true,
			resultPaths: nil,
			isValid:     false,
		},
	}
	for name, mtest := range mergeTests {
		t.Run(name, func(t *testing.T) {
			m1, err := ocfl.NewDigestMap(mtest.m1)
			if err != nil {
				t.Fatal("in test setup", err)
			}
			m2, err := ocfl.NewDigestMap(mtest.m2)
			if err != nil {
				t.Fatal("in test setup", err)
			}
			result, err := m1.Merge(m2, mtest.replace)
			if err != nil && mtest.isValid {
				t.Error("Merge() returned error for valid case: ", err)
			}
			if err == nil && !mtest.isValid {
				t.Error("Merge() returned no error for invalid case")
			}
			if mtest.isValid {
				resultPaths := result.PathMap()
				if !reflect.DeepEqual(result.PathMap(), mtest.resultPaths) {
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
		dm, err := ocfl.NewDigestMap(map[string][]string{
			"abc1": {"keep.txt", "delete.txt"},
			"abc2": {"save.txt"},
		})
		if err != nil {
			t.Fatal("in test setup:", err)
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

func TestMapMarshalJSON(t *testing.T) {
	a, _ := ocfl.NewDigestMap(map[string][]string{
		"abcd1": {"file.txt", "a/file2.txt"},
		"abcd2": {"a/b/data.csv"},
	})
	byts, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var b ocfl.DigestMap
	if err := json.Unmarshal(byts, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.PathMap(), b.PathMap()) {
		t.Logf("got: %v", b.PathMap())
		t.Logf("exp: %v", a.PathMap())
		t.Fatal("unmarshalled map doesn't included expected paths")
	}
	// Marshalling an invalid map should fail
	for desc, digests := range invalidMaps {
		t.Run(desc, func(t *testing.T) {
			// construct an invalid DigestMap by unmarshalling
			b, err := json.Marshal(digests)
			if err != nil {
				t.Fatal("during setup", err)
			}
			var m ocfl.DigestMap
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal("during setup", err)
			}
			if _, err := json.Marshal(m); err == nil {
				t.Error("invalid DigestMap marshalled without error")
			}
		})
	}
	// Marshalling a valid map should succeed
	for desc, digests := range validMaps {
		t.Run(desc, func(t *testing.T) {
			m, _ := ocfl.NewDigestMap(digests)
			if _, err := json.Marshal(m); err != nil {
				t.Fatal(err)
			}
		})
	}

}
