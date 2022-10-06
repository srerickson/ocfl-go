package ocfl_test

import (
	"errors"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
)

func TestIndex(t *testing.T) {
	idx := ocfl.NewIndex()
	info := &ocfl.IndexItem{
		SrcPaths: []string{"content.txt"},
	}
	if err := idx.Set("dir/a/tmp.txt", info); err != nil {
		t.Fatal(err)
	}
	entrs, err := idx.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if l := len(entrs); l != 1 {
		t.Fatalf("index has %d root entries, not 1", l)
	}
	if n := entrs[0].Name(); n != "dir" {
		t.Fatalf("tree root entry is %s, not %s", n, "dir")
	}
	inf, isdir, err := idx.Get("dir/a/tmp.txt")
	if err != nil {
		t.Fatal(err)
	}
	if isdir {
		t.Fatal("expected not directory")
	}
	if !inf.HasSrc("content.txt") {
		t.Fatal("expected 'content.txt'")
	}
	inf, isdir, err = idx.Get("dir/a")
	if err != nil {
		t.Fatal(err)
	}
	if !isdir {
		t.Fatal("expected a directory")
	}
	if inf != nil {
		t.Fatal("expected nil value")
	}
}

func TestIndexDiff(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		a, b := ocfl.NewIndex(), ocfl.NewIndex()
		if diff, _ := a.Diff(b, digest.SHA256); !diff.Equal() {
			t.Fatal("expected a,b to be equal")
		}
	})
	t.Run("same files", func(t *testing.T) {
		a, b := ocfl.NewIndex(), ocfl.NewIndex()
		addDigest(a, "a/b/c.txt", digest.SHA256, "abcdef")
		addDigest(b, "a/b/c.txt", digest.SHA256, "abcdef")
		if diff, _ := a.Diff(b, digest.SHA256); !diff.Equal() {
			t.Fatal("expected a,b to be equal")
		}
	})
	t.Run("empty, single addition", func(t *testing.T) {
		a, b := ocfl.NewIndex(), ocfl.NewIndex()
		addDigest(b, "a/b/c.txt", digest.SHA256, "abcdef")
		diff, err := a.Diff(b, digest.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		if diff.Equal() {
			t.Fatal("expected a,b not the equal")
		}
		if l := diff.Added.Len(); l != 1 {
			t.Fatal("expected 1 added file")
		}
		if diff.Removed != nil && diff.Removed.Len() > 0 {
			t.Fatal("expected no removed")
		}
		if diff.Changed != nil && diff.Changed.Len() > 0 {
			t.Fatal("expected no changed")
		}
		inf, isdir, err := diff.Added.Get("a/b/c.txt")
		if err != nil {
			t.Fatal(err)
		}
		if isdir {
			t.Fatal("expected file entry")
		}
		if inf.Digests[digest.SHA256] != "abcdef" {
			t.Fatal("expcted correct checksum")
		}
	})
	t.Run("single file removed", func(t *testing.T) {
		a, b := ocfl.NewIndex(), ocfl.NewIndex()
		addDigest(a, "a/b/c.txt", digest.SHA256, "abcdef")
		diff, err := a.Diff(b, digest.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		if diff.Equal() {
			t.Fatal("expected a,b not the equal")
		}
		if diff.Added != nil && diff.Added.Len() > 0 {
			t.Fatal("expected no addition")
		}
		if l := diff.Removed.Len(); l != 1 {
			t.Fatal("expected 1 added file")
		}
		if diff.Changed != nil && diff.Changed.Len() != 0 {
			t.Fatal("expected no addition")
		}
		inf, isdir, err := diff.Removed.Get("a/b/c.txt")
		if err != nil {
			t.Fatal(err)
		}
		if isdir {
			t.Fatal("expected file entry")
		}
		if inf.Digests[digest.SHA256] != "abcdef" {
			t.Fatal("expcted correct checksum")
		}
	})
	t.Run("single file changed", func(t *testing.T) {
		a, b := ocfl.NewIndex(), ocfl.NewIndex()
		addDigest(a, "a/b/c.txt", digest.SHA256, "abcdef1")
		addDigest(b, "a/b/c.txt", digest.SHA256, "abcdef2")
		diff, err := a.Diff(b, digest.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		if diff.Equal() {
			t.Fatal("expected a,b not the equal")
		}
		if diff.Added != nil && diff.Added.Len() > 0 {
			t.Fatal("expected no addition")
		}
		if diff.Removed != nil && diff.Removed.Len() > 0 {
			t.Fatal("expected no removed")
		}
		if l := diff.Changed.Len(); l != 1 {
			t.Fatal("expected one change")
		}
		inf, isdir, err := diff.Changed.Get("a/b/c.txt")
		if err != nil {
			t.Fatal(err)
		}
		if isdir {
			t.Fatal("expected file entry")
		}
		if inf.Digests[digest.SHA256] != "abcdef1" {
			t.Fatal("expected checksum from inital index")
		}
	})
	t.Run("combination", func(t *testing.T) {
		a, b := ocfl.NewIndex(), ocfl.NewIndex()
		addDigest(a, "a/b/removed1.txt", digest.SHA256, "abcdef0")
		addDigest(a, "a/b/removed2.txt", digest.SHA256, "abcdef1")
		addDigest(a, "a/b/c/unchanged.txt", digest.SHA256, "abcdef2")
		addDigest(b, "a/b/c/unchanged.txt", digest.SHA256, "abcdef2")
		addDigest(a, "a/b/changed.txt", digest.SHA256, "abcdef3")
		addDigest(b, "a/b/changed.txt", digest.SHA256, "abcdef4")
		addDigest(b, "a/b/added1.txt", digest.SHA256, "abcdef5")
		addDigest(b, "a/b/added2.txt", digest.SHA256, "abcdef6")
		addDigest(b, "a/b/added3.txt", digest.SHA256, "abcdef7")
		diff, err := a.Diff(b, digest.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		if diff.Equal() {
			t.Fatal("expected a,b not the equal")
		}
		if diff.Added == nil || diff.Added.Len() != 3 {
			t.Fatal("expected 3 additions")
		}
		if diff.Removed == nil || diff.Removed.Len() != 2 {
			t.Fatal("expected 2 removed")
		}
		if diff.Changed == nil || diff.Changed.Len() != 1 {
			t.Fatal("expected 1 change")
		}
	})

	t.Run("wrong digest", func(t *testing.T) {
		a, b := ocfl.NewIndex(), ocfl.NewIndex()
		addDigest(a, "a/b/c.txt", digest.SHA256, "abcdef")
		addDigest(b, "a/b/c.txt", digest.SHA256, "abcdef")
		if diff, _ := a.Diff(b, digest.SHA512); !diff.Equal() {
			t.Fatal("expected a,b to be equal")
		}
	})

}

func addDigest(idx *ocfl.Index, logical string, alg digest.Alg, sum string) error {
	n, isdir, err := idx.Get(logical)
	if err != nil && errors.Is(err, ocfl.ErrNotFound) {
		val := &ocfl.IndexItem{Digests: digest.Set{alg: sum}}
		return idx.Set(logical, val)
	}
	if err != nil {
		return err
	}
	if isdir {
		return ocfl.ErrNotFile
	}
	if n == nil {
		return errors.New("nil value on leaf node")
	}
	if n.Digests == nil {
		n.Digests = digest.Set{alg: sum}
		return nil
	}
	n.Digests[alg] = sum
	return nil
}
