package digest_test

import (
	"errors"
	"testing"

	"github.com/srerickson/ocfl/digest"
)

var invalidPaths = []string{"", "..", "/", "/file.txt", "dir//file.txt", "dir/"}

var testTrees = map[string]treeTest{
	`empty`: {},
	`empty-sha256`: {
		Alg: &digest.SHA256,
	},
	`single-file`: {
		Paths: map[string]string{"a.txt": "abcdef0123456789"},
	},
	`single-file-sha256`: {
		Paths: map[string]string{"a.txt": "abcdef0123456789"},
		Alg:   &digest.SHA256,
	},
	`dirs-sha256`: {
		Paths: map[string]string{
			"a.txt":       "a0123456789",
			"a/b.txt":     "ab0123456789",
			"a/b/c.txt":   "abc0123456789",
			"a/b/c/d.txt": "abcd0123456789",
		},
		Alg: &digest.SHA256,
	},
}

type treeTest struct {
	Paths map[string]string
	Alg   *digest.Alg
}

func (tt treeTest) Tree() *digest.Tree {
	tree := digest.Tree{}
	for p, d := range tt.Paths {
		if err := tree.SetDigest(p, d, false); err != nil {
			panic(err)
		}
	}
	return &tree
}

func testTreeSetDigest(t *testing.T, tt treeTest) {
	t.Helper()
	treeSize := len(tt.Paths)
	t.Run("invalid path error", func(t *testing.T) {
		tree := tt.Tree()
		for _, p := range invalidPaths {
			err := tree.SetDigest(p, "", false)
			if err == nil {
				t.Fatalf("expected an error for '%s'", p)
			}
			if !errors.Is(err, digest.ErrInvalidPath) {
				t.Fatalf("expected invalid path error for '%s'", p)
			}
		}
		if l := tree.Len(); l != treeSize {
			t.Fatalf("expected test tree to have size %d, got %d", treeSize, l)
		}
	})
	// SetDigest should return error if path is a directory
	t.Run("path not file error", func(t *testing.T) {
		tree := tt.Tree()
		if err := tree.SetDigest("dir/path", "abcd", false); err != nil {
			t.Fatal(err)
		}
		// replace=false
		for _, dir := range []string{".", "dir"} {
			err := tree.SetDigest(dir, "abcd", false)
			if err == nil {
				t.Fatalf("expected and error for %s", dir)
			}
			if !errors.Is(err, digest.ErrNotFile) {
				t.Fatal("expected ErrNotFile error")
			}
		}
		// should have one new path
		if l := tree.Len(); l != treeSize+1 {
			t.Fatalf("expted test tree to have size %d, got %d", treeSize+1, l)
		}
	})
	// SetDigest should return an error if replacing digest and replace is false
	t.Run("replacement", func(t *testing.T) {
		tree := tt.Tree()
		if err := tree.SetDigest("path", "abcd", false); err != nil {
			t.Fatal(err)
		}
		// replace=false
		err := tree.SetDigest("path", "efgh", false)
		if err == nil {
			t.Fatal("expected an error")
		}
		if !errors.Is(err, digest.ErrDigestExists) {
			t.Fatal("expected digest exists error")
		}
		// replace=true
		if err := tree.SetDigest("path", "efgh", true); err != nil {
			t.Fatal(err)
		}
		// should have one new path
		if l := tree.Len(); l != treeSize+1 {
			t.Fatalf("expted test tree to have size %d, got %d", treeSize+1, l)
		}
	})

}

func testTreeGetDigest(t *testing.T, tt treeTest) {
	t.Helper()
	//treeSize := len(tt.Files)
	// GetDigest should return the digest for existing files
	t.Run("existing file", func(t *testing.T) {
		tree := tt.Tree()
		for f, expect := range tt.Paths {
			got, err := tree.GetDigest(f)
			if err != nil {
				t.Fatal(err)
			}
			if got != expect {
				t.Fatalf("expected digest to be %s, got %s", expect, got)
			}
		}
	})
	// GetDigest should return the digest for existing directories (recursive)
	// t.Run("existing directories", func(t *testing.T) {
	// 	tree := tt.Tree()
	// 	if tree.DigestAlg == nil {
	// 		return
	// 	}
	// 	for f := range tt.Paths {
	// 		dir := path.Dir(f)
	// 		d, err := tree.GetDigest(dir, true)
	// 		if err != nil {
	// 			t.Fatal(err)
	// 		}
	// 		if d == "" {
	// 			t.Fatalf("returned digest is empty")
	// 		}
	// 	}
	// })
	// GetDigest should return an error for invalid paths
	t.Run("invalid path error", func(t *testing.T) {
		tree := tt.Tree()
		for _, p := range invalidPaths {
			_, err := tree.GetDigest(p)
			if err == nil {
				t.Fatalf("expected an error for '%s'", p)
			}
			if !errors.Is(err, digest.ErrInvalidPath) {
				t.Fatalf("expected invalid path error for '%s'", p)
			}
		}
	})
	// GetDigest should respect recursive param
	// t.Run("digest alg error", func(t *testing.T) {
	// 	tree := tt.Tree()
	// 	if tree.DigestAlg == nil {
	// 		_, err := tree.GetDigest(".", true)
	// 		if err == nil {
	// 			t.Fatalf("expected an error")
	// 		}
	// 		if !errors.Is(err, digest.ErrDigestAlg) {
	// 			t.Fatalf("expected digest alg error, got '%v'", err)
	// 		}
	// 	}
	// })
	// GetDigest should return error for missing paths
	t.Run("not found error", func(t *testing.T) {
		tree := tt.Tree()
		_, err := tree.GetDigest("a/b/none.txt")
		if err == nil {
			t.Fatal("expected an error")
		}
		if !errors.Is(err, digest.ErrNotFound) {
			t.Fatalf("expected not found error, got %s", err)
		}
	})
}

func testTreeRemove(t *testing.T, tt treeTest) {
	t.Helper()
	//treeSize := len(tt.Paths)
	t.Run("remove files", func(t *testing.T) {
		tree := tt.Tree()
		for f := range tt.Paths {
			size := tree.Len()
			if err := tree.Remove(f, false); err != nil {
				t.Fatal(err)
			}
			newSize := tree.Len()
			if newSize != size-1 {
				t.Fatal("expected size to be less by 1")
			}
		}
		// tree should be empty
		enries, err := tree.DirEntries(".")
		if err != nil {
			t.Fatal(err)
		}
		if len(enries) != 0 {
			t.Fatal("expected tree to be empty")
		}
	})

	t.Run("remove dirs", func(t *testing.T) {
		tree := tt.Tree()
		// all tests should have one file
		if err := tree.SetDigest("a-new.txt", "000000", true); err != nil {
			t.Fatal(err)
		}
		if err := tree.SetDigest("a/new.txt", "012345", false); err != nil {
			t.Fatal(err)
		}
		if err := tree.Remove("a", true); err != nil {
			t.Fatal(err)
		}
		entries, err := tree.DirEntries(".")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 0 {
			t.Fatal("deleted more than the directory")
		}
		for _, e := range entries {
			if e.Name() == "a" {
				t.Fatal("directory still exists")
			}
		}
	})

	t.Run("not found error", func(t *testing.T) {
		tree := tt.Tree()
		size := tree.Len()
		err := tree.Remove("a-new.txt", false)
		if err == nil {
			t.Fatal("expected an error")
		}
		if !errors.Is(err, digest.ErrNotFound) {
			t.Fatalf("expected not found error, got %s", err)
		}
		if s := tree.Len(); s != size {
			t.Fatal("tree size changed")
		}

	})

	t.Run("invalid path error", func(t *testing.T) {
		tree := tt.Tree()
		for _, p := range invalidPaths {
			size := tree.Len()
			err := tree.Remove(p, true)
			if err == nil {
				t.Fatalf("expected an error for '%s'", p)
			}
			if !errors.Is(err, digest.ErrInvalidPath) {
				t.Fatalf("expected invalid path error for '%s'", p)
			}
			if s := tree.Len(); s != size {
				t.Fatal("tree size changed")
			}
		}
	})

}

func TestTree(t *testing.T) {
	for name, ttest := range testTrees {
		t.Run(name, func(t *testing.T) {
			t.Run("SetDigest", func(t *testing.T) {
				testTreeSetDigest(t, ttest)
			})
			t.Run("GetDigest", func(t *testing.T) {
				testTreeGetDigest(t, ttest)
			})
			t.Run("Remove", func(t *testing.T) {
				testTreeRemove(t, ttest)
			})
		})
	}
}
