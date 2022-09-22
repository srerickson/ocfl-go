package pathtree_test

import (
	"errors"
	"testing"

	"github.com/srerickson/ocfl/internal/pathtree"
)

func newPathTree(paths map[string]string) (*pathtree.Node[string], error) {
	tree := pathtree.NewDir[string]()
	for p, v := range paths {
		if err := pathtree.SetVal(tree, p, v, false); err != nil {
			return nil, err
		}
	}

	return tree, nil
}

func TestSet(t *testing.T) {
	tree, err := newPathTree(map[string]string{
		"a/b/c.txt":  "content",
		"a/b/c2.txt": "content2",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Set existing w/o replace
	err = pathtree.Set(tree, "a", pathtree.NewDir[string](), false)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, pathtree.ErrValueExists) {
		t.Fatalf("expected error to be ErrValueExists, not %v", err)
	}
	// Set existing w/ replace
	err = pathtree.Set(tree, "a", pathtree.NewDir[string](), true)
	if err != nil {
		t.Fatal(err)
	}
	node, err := pathtree.Get(tree, "a")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(pathtree.Children(node)); l != 0 {
		t.Fatalf("expected zero entries, not %d", l)
	}
	if l := len(pathtree.Children(tree)); l != 1 {
		t.Fatalf("expected 1 entry, not %d", l)
	}
}

func TestGet(t *testing.T) {
	tree, err := newPathTree(map[string]string{
		"a/b/c.txt": "content",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"a", "a/b"} {
		node, err := pathtree.Get(tree, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !pathtree.IsDir(node) {
			t.Fatal("expected node to be a directory")
		}
		if node.Val != "" {
			t.Fatal("expected empty string")
		}
	}
	node, err := pathtree.Get(tree, "a/b/c.txt")
	if err != nil {
		t.Fatal(err)
	}
	if pathtree.IsDir(node) {
		t.Fatal("expected a value node")
	}
	if node.Val != "content" {
		t.Fatal("unexpected value", node.Val)
	}
	// missing
	_, err = pathtree.Get(tree, "a/b/c.txt/d")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, pathtree.ErrNotFound) {
		t.Fatalf("expected error to be ErrNotFound, not %v", err)
	}
}

func TestMkdirAll(t *testing.T) {
	tree := pathtree.NewDir[string]()
	n, err := pathtree.MkdirAll(tree, "a/b/c")
	if err != nil {
		t.Fatal(err)
	}
	if !pathtree.IsDir(n) {
		t.Fatal("expected node to be a directory")
	}
	for _, dir := range []string{"a", "a/b", "a/b/c"} {
		node, err := pathtree.Get(tree, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !pathtree.IsDir(node) {
			t.Fatal("expected node to be a directory")
		}
		if node.Val != "" {
			t.Fatal("expected empty string")
		}
	}
	if err := pathtree.SetVal(tree, "a/b/c.txt", "a value", false); err != nil {
		t.Fatal(err)
	}
	_, err = pathtree.MkdirAll(tree, "a/b/c.txt/d")
	if err == nil {
		t.Fatal("expected an error")
	}

}
