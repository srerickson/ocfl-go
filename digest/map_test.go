package digest

import (
	"reflect"
	"testing"
)

func TestParentDirs(t *testing.T) {
	type comp struct {
		in  string
		out []string
	}
	table := []comp{
		{"", nil},
		{"/", nil},
		{"a/", nil},
		{"/a", nil},
		{"a/b/", nil},
		{"/a/b/c/d", nil},
		{"a", []string{"."}},
		{"a/b", []string{"a"}},
		{"a/b", []string{"a"}},
		{"a/b/c", []string{"a", "a/b"}},
	}
	for _, row := range table {
		got, err := parentDirs(row.in)
		if row.out == nil && err == nil {
			t.Errorf("expected error for %s", row.in)
		}
		if !reflect.DeepEqual(got, row.out) {
			t.Errorf("expected parentDirs(\"%s\") to be %v, got %v", row.in, row.out, got)
		}
	}
}

func TestValidPath(t *testing.T) {
	type comp struct {
		path  string
		valid bool
	}
	table := []comp{
		{"a", true},
		{"a/b/c.txt", true},
		{"a/b/c.txt", true},
		{"", false},
		{".", false},
		{"..", false},
		{"//", false},
		{"./a", false},
		{"../a", false},
		{"a/b//c", false},
		{"a/b/./c", false},
		{"a/b/../c", false},
		{"a/b/c/..", false},
		{"/a/b/c.txt", false},
	}
	for _, row := range table {
		v := validPath(row.path)
		if v != row.valid {
			if row.valid {
				t.Errorf("expected validPath(\"%s\") to return no error", row.path)
			} else {
				t.Errorf("expected validPath(\"%s\") to return an error", row.path)
			}
		}

	}
}

func TestDigestMapAdd(t *testing.T) {
	dm := NewMap()
	err := dm.Add(`a1b2b3b`, `file-1.txt`)
	if err != nil {
		t.Error(err)
	}
	err = dm.Add(`a1b2b3b`, `file-2.txt`)
	if err != nil {
		t.Error(err)
	}
	err = dm.Add(`fe1b2b3b`, `file-1.txt`)
	if err == nil {
		t.Error("Expected error adding duplicate file")
	}
	err = dm.Add(`A1B2B3B`, `file-3.txt`)
	if err == nil {
		t.Error("Expected error adding different version of existing digest")
	}
	paths := dm.AllPaths()
	if _, exists := paths[`file-1.txt`]; !exists {
		t.Error("Expected file-1.txt to exist")
	}
	if _, exists := paths[`file-2.txt`]; !exists {
		t.Error("Expected file-2.txt to exist")
	}
}

// func TestDigestMapMerge(t *testing.T) {

// 	var dm1, dm2, dm3 Map

// 	json.NewDecoder(strings.NewReader(`{
// 		"abc": ["file-1.txt"],
// 		"123": ["file-2.txt"],
// 		"cdf": ["a/file.txt", "b/file.txt"]
// 	}`)).Decode(&dm1)

// 	json.NewDecoder(strings.NewReader(`{
// 		"123": ["file-2"],
// 		"xyz": ["c/file.txt"]
// 	}`)).Decode(&dm2)

// 	json.NewDecoder(strings.NewReader(`{
// 		"bad": ["file-2"]
// 	}`)).Decode(&dm3)

// 	err := dm1.Merge(&dm2)
// 	if err != nil {
// 		t.Error(err)
// 	}
// 	if dm1.GetDigest(`c/file.txt`) != "xyz" {
// 		t.Error(`expected c/file.txt to be merged`)
// 	}
// 	err = dm1.Merge(&dm3)
// 	if err == nil {
// 		t.Error(`expected an error`)
// 	}

// }
