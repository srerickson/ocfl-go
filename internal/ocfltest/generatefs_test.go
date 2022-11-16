package ocfltest_test

import (
	"context"
	"io/fs"
	"math/rand"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/digest/checksum"
	"github.com/srerickson/ocfl/internal/ocfltest"
)

const (
	expectedSum = "6056876ce029e3e2b242921a13e47042ab2ce93449615c813b868620608e679c"
	// the sum is deterministic if all the following value remain the same
	seed     = 7382892873
	numfiles = 300
	maxsize  = 1024 * 1024
)

func TestGenerateFS(t *testing.T) {
	genr := rand.New(rand.NewSource(seed))
	fsys := ocfltest.GenerateFS(genr, numfiles, maxsize)
	found := 0
	gotMax := 0
	walkfn := func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.Type().IsRegular() {
			found++
			inf, err := e.Info()
			if err != nil {
				return err
			}
			if inf.Size() > int64(gotMax) {
				gotMax = int(inf.Size())
			}
		}
		return nil
	}
	if err := fs.WalkDir(fsys, ".", walkfn); err != nil {
		t.Fatal(err)
	}
	if found != numfiles {
		t.Fatalf("expected %d files, found %d", numfiles, found)
	}
	if gotMax > maxsize {
		t.Fatalf("expected no files larger than %d", maxsize)
	}
	index, err := ocfl.IndexDir(context.Background(), ocfl.NewFS(fsys), ".", checksum.WithAlgs(digest.SHA256()))
	if err != nil {
		t.Fatal(err)
	}
	if err := index.SetDirDigests(digest.SHA256()); err != nil {
		t.Fatal(err)
	}
	rootVal, _, err := index.Get(".")
	if err != nil {
		t.Fatal(err)
	}
	sum := rootVal.Digests[digest.SHA256id]
	if sum != expectedSum {
		t.Fatal("recursive digest of generated fs is not the expected value.")
	}
}
