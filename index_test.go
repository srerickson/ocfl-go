package ocfl_test

import (
	"errors"
	"path"
	"reflect"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/pathtree"
)

func newTestIndex(paths map[string]ocfl.IndexItem) (*ocfl.Index, error) {
	root := pathtree.NewDir[ocfl.IndexItem]()
	for p, item := range paths {
		if err := root.SetFile(p, item); err != nil {
			return nil, err
		}
	}
	idx := &ocfl.Index{}
	idx.SetRoot(root)
	return idx, nil
}

func TestIndexGetVal(t *testing.T) {
	testPaths := map[string]ocfl.IndexItem{
		"file1.txt": {
			Digests:  digest.Set{digest.SHA256id: "abcdef1234567890"},
			SrcPaths: []string{"v1/content/file.txt"},
		},
		"a/file2.txt": {
			Digests:  digest.Set{digest.SHA256id: "abcdef1234567890"},
			SrcPaths: []string{"v1/content/file.txt"},
		},
		"a/b/file3.txt": {
			Digests:  digest.Set{digest.SHA256id: "abcdef1234567890"},
			SrcPaths: []string{"v1/content/file3.txt"},
		},
	}
	idx, err := newTestIndex(testPaths)
	if err != nil {
		t.Fatal(err)
	}
	for p, expected := range testPaths {
		item, isdir, err := idx.GetVal(p)
		if err != nil {
			t.Fatal(err)
		}
		if isdir {
			t.Fatalf("expected isdir to be false for '%s'", p)
		}
		if !reflect.DeepEqual(item, expected) {
			t.Fatalf("index digests don't match expected value")
		}
		_, isdir, err = idx.GetVal(path.Dir(p))
		if err != nil {
			t.Fatal(err)
		}
		if !isdir {
			t.Fatalf("expected isdir to to be true for '%s'", path.Dir(p))
		}
	}

	t.Run("with invalid paths", func(t *testing.T) {
		invalids := []string{"..", "", "a//b.txt"}
		for _, p := range invalids {
			if _, _, err := idx.GetVal(p); !errors.Is(err, ocfl.ErrInvalidPath) {
				t.Fatalf("expected invalid path error for '%s', got: %s", p, err.Error())
			}
		}
	})

	t.Run("with missing paths", func(t *testing.T) {
		missing := []string{"missing", "a/nothere.txt", "a/b/c/nope.txt"}
		for _, p := range missing {
			if _, _, err := idx.GetVal(p); !errors.Is(err, ocfl.ErrNotFound) {
				t.Fatalf("expected invalid path error for '%s', got: %s", p, err.Error())
			}
		}
	})
}
