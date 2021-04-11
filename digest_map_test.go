package ocfl

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
		{"a", nil},
		{"a/", []string{"a"}},
		{"/a", nil},
		{"/a/b/c/d", []string{"/a", "/a/b", "/a/b/c"}},
		{"a/b", []string{"a"}},
		{"a/b", []string{"a"}},
		{"a/b/c", []string{"a", "a/b"}},
		{"a/b/", []string{"a", "a/b"}},
	}
	for _, row := range table {
		got := parentDirs(row.in)
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
