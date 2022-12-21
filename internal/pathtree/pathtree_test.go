package pathtree_test

import (
	"errors"
	"sort"
	"testing"

	"github.com/srerickson/ocfl/internal/pathtree"
)

func newPathTree(paths map[string]string) (*pathtree.Node[string], error) {
	tree := pathtree.NewDir[string]()
	for p, v := range paths {
		if err := tree.Set(p, pathtree.NewFile(v)); err != nil {
			return nil, err
		}
	}

	return tree, nil
}

func TestSet(t *testing.T) {

	empty := pathtree.Node[string]{}
	err := empty.Set(".", pathtree.NewFile("test"))
	if err != nil {
		t.Fatal(err)
	}

	tree, err := newPathTree(map[string]string{
		"a/b/c.txt":  "content",
		"a/b/c2.txt": "content2",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Set existing
	err = tree.Set("a", pathtree.NewDir[string]())
	if err != nil {
		t.Fatal(err)
	}
	node, err := tree.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(node.DirEntries()); l != 0 {
		t.Fatalf("expected zero entries, not %d", l)
	}
	if l := len(tree.DirEntries()); l != 1 {
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
	err = tree.Set(`a`, sub)
	if err == nil {
		t.Fatal("expect an error")
	}
	if !errors.Is(err, pathtree.ErrRelation) {
		t.Fatal("expected ErrRelation")
	}
	err = tree.Set(`a/b/c`, sub)
	if err == nil {
		t.Fatal("expect an error")
	}
	if !errors.Is(err, pathtree.ErrRelation) {
		t.Fatal("expected ErrRelation")
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
	tree := pathtree.NewDir[string]()
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
	if err := tree.Set("a/b/c.txt", pathtree.NewFile("a value")); err != nil {
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
	sub, err := tree.Remove("a/b1")
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

func TestDirEntries(t *testing.T) {
	tree, err := newPathTree(map[string]string{
		"zebra/file1.txt": "content",
		"file2.txt":       "content2",
		"a/file2.txt":     "content2",
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := tree.DirEntries()
	if len(entries) != 3 {
		t.Fatal("expected 3 entries")
	}
	if !sort.IsSorted(pathtree.DirEntries(entries)) {
		t.Fatal("expected entries to be sorted")
	}
}

func TestCopy(t *testing.T) {
	tree, err := newPathTree(map[string]string{
		"zebra/file1.txt": "content",
		"zebra/file2.txt": "content2",
		"file2.txt":       "content3",
		"a/file2.txt":     "content3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if l := tree.Len(); l != 4 {
		t.Fatal("expected Len() to be 4")
	}
	cp := tree.Copy()
	if l := cp.Len(); l != 4 {
		t.Fatal("expected Len() to be 4")
	}
	if _, err := cp.Get("zebra/file1.txt"); err != nil {
		t.Fatal(err)
	}
	if l := tree.Len(); l != 4 {
		t.Fatal("expected Len() to be 4")
	}
	if cp.IsParentOf(tree) || tree.IsParentOf(cp) {
		t.Fatal("copy is still related to tree")
	}
}
