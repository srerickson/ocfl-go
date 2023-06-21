package digest_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/srerickson/ocfl/digest"
)

var invalidPaths = []string{
	".",
	"/file1.txt",
	"../file1.txt",
	"./file.txt",
	"dir//file.txt",
	"dir/./file.txt",
	"dir/../file.txt",
}

var validMaps = map[string]map[string][]string{
	"empty": {},
	"single file": {
		"abcde": {"file.txt"},
	},
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

func testMapValid(t *testing.T, desc string, m *digest.Map, expOK bool) {
	t.Helper()
	t.Run(desc, func(t *testing.T) {
		err := m.Valid()
		if err == nil && !expOK {
			t.Fatal("invalid map was found to be valid")
		}
		if err != nil && expOK {
			t.Fatalf("valid map was found to be invalid, with error: %s", err)
		}
	})

}

func testMapMakerAdd(t *testing.T, desc string, mm *digest.MapMaker, digest string, pth string, expOK bool) {
	t.Run(desc, func(t *testing.T) {
		err := mm.Add(digest, pth)
		if err == nil && !expOK {
			t.Fatal("error was expected but none was returned")
		}
		if err != nil && expOK {
			t.Fatal(err)
		}
		if err := mm.Map().Valid(); err != nil {
			t.Fatal("the resuling map is not valid")
		}
		if expOK {
			// expect difest to be in the map
			if !mm.HasDigest(digest) {
				t.Fatal("the digest was not added")
			}
		}

	})
}

func TestMapValid(t *testing.T) {
	for _, p := range invalidPaths {
		desc := "invalid path: " + p
		m := digest.NewMapUnsafe(map[string][]string{
			"abcd": {p},
		})
		testMapValid(t, desc, m, false)
	}
	for desc, digests := range invalidMaps {
		testMapValid(t, desc, digest.NewMapUnsafe(digests), false)
	}
	for desc, digests := range validMaps {
		testMapValid(t, desc, digest.NewMapUnsafe(digests), true)
	}
}

func TestMapMakerAdd(t *testing.T) {
	// Adding Invalid Paths
	for _, p := range invalidPaths {
		desc := "invalid path: " + p
		empty := &digest.MapMaker{}
		testMapMakerAdd(t, desc, empty, "abcd", p, false)
	}
	// Adding digest/paths from valid maps
	for desc, digests := range validMaps {
		for d, paths := range digests {
			for _, p := range paths {
				empty := &digest.MapMaker{}
				testMapMakerAdd(t, desc, empty, d, p, true)
			}
		}
	}
	// Adding to existing
	mm, err := digest.MapMakerFrom(*digest.NewMapUnsafe(map[string][]string{
		"abcd1": {"file.txt", "a/file2.txt"},
		"abcd2": {"a/b/data.csv"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	// these are ok
	testMapMakerAdd(t, "new file, new digest", mm, "abcd3", "newfile1.txt", true)
	testMapMakerAdd(t, "new file, existing digest", mm, "abcd3", "newfile2.txt", true)
	testMapMakerAdd(t, "new file, existing digest uppercase", mm, "ABCD3", "newfile3.txt", true)
	// errors
	testMapMakerAdd(t, "path conflict", mm, "abcd4", "newfile2.txt", false)
	testMapMakerAdd(t, "path conflict again", mm, "abcd4", "a/b", false)

	m := mm.Map()
	expectPaths := map[string]string{
		"file.txt":     "abcd1",
		"a/file2.txt":  "abcd1",
		"a/b/data.csv": "abcd2",
		"newfile1.txt": "abcd3",
		"newfile2.txt": "abcd3",
		"newfile3.txt": "abcd3",
	}
	if !reflect.DeepEqual(m.AllPaths(), expectPaths) {
		t.Logf("got: %v", m.AllPaths())
		t.Logf("exp: %v", expectPaths)
		t.Fatal("map doesn't included expected paths")
	}
}

func TestMapMarshalJSON(t *testing.T) {
	a := digest.NewMapUnsafe(map[string][]string{
		"abcd1": {"file.txt", "a/file2.txt"},
		"abcd2": {"a/b/data.csv"},
	})
	byts, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var b digest.Map
	if err := json.Unmarshal(byts, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.AllPaths(), b.AllPaths()) {
		t.Logf("got: %v", b.AllPaths())
		t.Logf("exp: %v", a.AllPaths())
		t.Fatal("unmarshalled map doesn't included expected paths")
	}
	// Marshalling an invalid map should fail
	for desc, digests := range invalidMaps {
		t.Run(desc, func(t *testing.T) {
			if _, err := json.Marshal(digest.NewMapUnsafe(digests)); err == nil {
				t.Fatal("invalid map marshalled without error")
			}
		})
	}
	// Marshalling a valid map should succeed
	for desc, digests := range validMaps {
		t.Run(desc, func(t *testing.T) {
			if _, err := json.Marshal(digest.NewMapUnsafe(digests)); err != nil {
				t.Fatal(err)
			}
		})
	}

}

func TestMapMerge(t *testing.T) {
	base := digest.NewMapUnsafe(map[string][]string{
		"1234":   {"datafile.txt"},
		"abcde1": {"newfile.txt"},
	})
	for desc, digests := range validMaps {
		t.Run("valid map: "+desc, func(t *testing.T) {
			m2 := digest.NewMapUnsafe(digests)
			merged, err := base.Merge(m2)
			if err != nil {
				t.Fatal(err)
			}
			for p, expectDigest := range base.AllPaths() {
				gotDigest := merged.GetDigest(p)
				if gotDigest != expectDigest {
					t.Fatalf("merged digest for '%s' is '%s', expected '%s'", p, gotDigest, expectDigest)
				}
			}
			for p, expectDigest := range m2.AllPaths() {
				gotDigest := merged.GetDigest(p)
				if gotDigest != expectDigest {
					t.Fatalf("merged digest for '%s' is '%s', expected '%s'", p, gotDigest, expectDigest)
				}
			}
		})
	}
	for desc, digests := range invalidMaps {
		t.Run("invalid map: "+desc, func(t *testing.T) {
			m2 := digest.NewMapUnsafe(digests)
			if _, err := base.Merge(m2); err == nil {
				t.Fatal("expected an errot")
			}
		})
	}

}
