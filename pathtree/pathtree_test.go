package pathtree_test

import (
	"errors"
	"sort"
	"testing"

	"github.com/srerickson/ocfl/pathtree"
)

func newPathTree(paths map[string]string) (*pathtree.Node[string], error) {
	tree := pathtree.NewRoot[string]()
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
	err = tree.Set("a", pathtree.NewRoot[string](), false)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, pathtree.ErrValueExists) {
		t.Fatalf("expected error to be ErrValueExists, not %v", err)
	}
	// Set existing w/ replace
	err = tree.Set("a", pathtree.NewRoot[string](), true)
	if err != nil {
		t.Fatal(err)
	}
	node, err := tree.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(node.ReadDir()); l != 0 {
		t.Fatalf("expected zero entries, not %d", l)
	}
	if l := len(tree.ReadDir()); l != 1 {
		t.Fatalf("expected 1 entry, not %d", l)
	}
	// prevent cycle
	tree, err = newPathTree(map[string]string{
		"a/b/c/file1.txt": "content",
		"a/b/c/file2.txt": "content2",
	})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := tree.Get(`a/b`)
	if err != nil {
		t.Fatal(err)
	}
	err = tree.Set(`a`, sub, true)
	if err == nil {
		t.Fatal("expect an error")
	}
	if !errors.Is(err, pathtree.ErrCycle) {
		t.Fatal("expected ErrCycle")
	}
	err = tree.Set(`a/b/c`, sub, true)
	if err == nil {
		t.Fatal("expect an error")
	}
	if !errors.Is(err, pathtree.ErrCycle) {
		t.Fatal("expected ErrCycle")
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
		node, err := tree.Get(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !node.IsDir() {
			t.Fatal("expected node to be a directory")
		}
		if node.Val != "" {
			t.Fatal("expected empty string")
		}
	}
	node, err := tree.Get("a/b/c.txt")
	if err != nil {
		t.Fatal(err)
	}
	if node.IsDir() {
		t.Fatal("expected a value node")
	}
	if node.Val != "content" {
		t.Fatal("unexpected value", node.Val)
	}
	// missing
	_, err = tree.Get("a/b/c.txt/d")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, pathtree.ErrNotFound) {
		t.Fatalf("expected error to be ErrNotFound, not %v", err)
	}
}

func TestMkdirAll(t *testing.T) {
	tree := pathtree.NewRoot[string]()
	n, err := tree.MkdirAll("a/b/c")
	if err != nil {
		t.Fatal(err)
	}
	if !n.IsDir() {
		t.Fatal("expected node to be a directory")
	}
	for _, dir := range []string{"a", "a/b", "a/b/c"} {
		node, err := tree.Get(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !node.IsDir() {
			t.Fatal("expected node to be a directory")
		}
		if node.Val != "" {
			t.Fatal("expected empty string")
		}
	}
	if err := pathtree.SetVal(tree, "a/b/c.txt", "a value", false); err != nil {
		t.Fatal(err)
	}
	_, err = tree.MkdirAll("a/b/c.txt/d")
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestRemove(t *testing.T) {
	tree, err := newPathTree(map[string]string{
		"a/b1/c/file1.txt": "content",
		"a/b2/c/file2.txt": "content2",
	})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := tree.Remove("a/b1", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Get(`a/b2/c/file2.txt`); err != nil {
		t.Fatal(err)
	}
	if _, err := sub.Get(`c/file1.txt`); err != nil {
		t.Fatal(err)
	}
	if tree.IsParentOf(sub) {
		t.Fatal("expected sub to not be part of tree")
	}
}

func TestReadDir(t *testing.T) {
	tree, err := newPathTree(map[string]string{
		"zebra/file1.txt": "content",
		"file2.txt":       "content2",
		"a/file2.txt":     "content2",
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := tree.ReadDir()
	if len(entries) != 3 {
		t.Fatal("expected 3 entries")
	}
	if !sort.IsSorted(pathtree.DirEntries(entries)) {
		t.Fatal("expected entries to be sorted")
	}
}

func TestHasCycle(t *testing.T) {
	tree, err := newPathTree(map[string]string{
		"a/b/c/d.txt": "-",
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := newPathTree(map[string]string{
		"cycle.txt": "-",
	})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := tree.Get("a/b")
	if err != nil {
		t.Fatal(err)
	}
	// add child tree as "a2"
	if err := tree.Set("a2", child, false); err != nil {
		t.Fatal(err)
	}
	// add child tree as a/b/c2
	if err := sub.Set("c2", child, false); err != nil {
		t.Fatal(err)
	}
	if !tree.HasCycle() {
		t.Fatal("expected true")
	}
	// removing to fix
	if _, err := tree.Remove("a/b/c2", true); err != nil {
		t.Fatal(err)
	}
	if tree.HasCycle() {
		t.Fatal("expected no cycle")
	}
}
