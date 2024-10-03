package ocfl_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	"golang.org/x/crypto/blake2b"
)

func TestDigester(t *testing.T) {
	testDigester := func(t *testing.T, algs []string) {
		data := []byte("content")
		t.Helper()
		dig := ocfl.NewMultiDigester(algs...)
		// readfrom
		if _, err := io.Copy(dig, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		result := dig.Sums()
		if l := len(result); l != len(algs) {
			t.Fatalf("Sums() returned wrong number of entries: got=%d, expect=%d", l, len(algs))
		}
		expect := ocfl.DigestSet{}
		for alg := range result {
			var err error
			expect[alg], err = testAlg(alg, data)
			if err != nil {
				t.Error(err)
			}
			if expect[alg] != result[alg] {
				t.Errorf("ReadFrom() has unexpected result for %s: got=%q, expect=%q", alg, dig.Sum(alg), expect[alg])
			}
		}
		if err := result.Validate(bytes.NewReader(data)); err != nil {
			t.Error("Validate() unexpected error:", err)
		}
		// add invalid entry
		result[digest.SHA1.ID()] = "invalid"
		err := result.Validate(bytes.NewReader(data))
		if err == nil {
			t.Error("Validate() didn't return an error for an invalid DigestSet")
		}
		var digestErr *ocfl.DigestError
		if !errors.As(err, &digestErr) {
			t.Error("Validate() didn't return a DigestErr as expected")
		}
		if digestErr.Alg != digest.SHA1.ID() {
			t.Error("Validate() returned an error with the wrong Alg value")
		}
		if digestErr.Expected != result[digest.SHA1.ID()] {
			t.Error("Validate() returned an error with wrong Expected value")
		}
	}
	t.Run("nil algs", func(t *testing.T) {
		testDigester(t, nil)
	})
	t.Run("1 alg", func(t *testing.T) {
		testDigester(t, []string{digest.MD5.ID()})
	})
	t.Run("2 algs", func(t *testing.T) {
		testDigester(t, []string{digest.MD5.ID(), digest.BLAKE2B.ID()})
	})
}

func testAlg(algID string, val []byte) (string, error) {
	var h hash.Hash
	switch algID {
	case digest.SHA512.ID():
		h = sha512.New()
	case digest.SHA256.ID():
		h = sha256.New()
	case digest.SHA1.ID():
		h = sha1.New()
	case digest.MD5.ID():
		h = md5.New()
	case digest.BLAKE2B.ID():
		h, _ = blake2b.New512(nil)
	}
	alg, _ := digest.Defaults.New(algID)
	d := alg.Digester()
	d.Write(val)
	h.Write(val)
	exp := hex.EncodeToString(h.Sum(nil))
	got := d.String()
	if exp != got {
		return exp, fmt.Errorf("%s value: got=%q, expected=%q", algID, got, exp)
	}
	return exp, nil
}

func TestDigestFS(t *testing.T) {
	var testMD5Sums = map[string]string{
		"hello.csv": "9d02fa6e9dd9f38327f7b213daa28be6",
	}
	fsys := ocfl.DirFS(filepath.Join("testdata", "content-fixture"))
	ctx := context.Background()
	t.Run("no input", func(t *testing.T) {
		setup := func(add func(name string, algs []string) bool) {}
		for range ocfl.Digest(ctx, fsys, setup) {
			t.Error("shouldn't be called")
		}
	})
	t.Run("missing file", func(t *testing.T) {
		setup := func(add func(name string, algs []string) bool) {
			add(filepath.Join("missingfile"), []string{digest.MD5.ID()})
		}
		for _, err := range ocfl.Digest(ctx, fsys, setup) {
			be.True(t, err != nil)
		}
	})
	t.Run("unknown alg", func(t *testing.T) {
		setup := func(add func(name string, algs []string) bool) {
			add("hello.csv", []string{"bad"})
		}
		for digests, err := range ocfl.Digest(ctx, fsys, setup) {
			be.NilErr(t, err)
			be.Zero(t, len(digests.Digests))
		}
	})
	t.Run("minimal, one existing file, md5", func(t *testing.T) {
		setup := func(add func(name string, algs []string) bool) {
			add("hello.csv", []string{digest.MD5.ID()})
		}
		for r, err := range ocfl.Digest(ctx, fsys, setup) {
			be.NilErr(t, err)
			be.Equal(t, testMD5Sums[r.Path], r.Digests[digest.MD5.ID()])
		}
	})
	t.Run("multiple files, md5, sha1", func(t *testing.T) {
		algs := []string{digest.MD5.ID(), digest.SHA1.ID()}
		setup := func(add func(name string, algs []string) bool) {
			add("hello.csv", algs)
			add("folder1/file.txt", algs)
			add("folder1/folder2/file2.txt", algs)
			add("folder1/folder2/sculpture-stone-face-head-888027.jpg", algs)
		}
		for r, err := range ocfl.Digest(ctx, fsys, setup) {
			be.NilErr(t, err)
			be.Nonzero(t, r.Digests[digest.MD5.ID()])
			be.Nonzero(t, r.Digests[digest.SHA1.ID()])
		}
	})
	t.Run("minimal, one existing file, no algs", func(t *testing.T) {
		setup := func(add func(name string, algs []string) bool) {
			add("hello.csv", nil)
		}
		for r, err := range ocfl.Digest(ctx, fsys, setup) {
			be.NilErr(t, err)
			be.Zero(t, len(r.Digests))
		}
	})
}

func TestPathDigests(t *testing.T) {
	ctx := context.Background()
	fsys := fstest.MapFS{
		"data/sample.txt":        &fstest.MapFile{Data: []byte("some content1")},
		"data/sample2.txt":       &fstest.MapFile{Data: []byte("some content2")},
		"data/sample3.txt":       &fstest.MapFile{Data: []byte("some content3")},
		"data/sample4.txt":       &fstest.MapFile{Data: []byte("some content4")},
		"data/sample5.txt":       &fstest.MapFile{Data: []byte("some content5")},
		"data/subdir/sample.txt": &fstest.MapFile{Data: []byte("some content6")},
	}
	t.Run("example", func(t *testing.T) {
		jobs := func(yield func(string, []string) bool) {
			for name := range fsys {
				yield(name, []string{digest.SHA256.ID(), digest.MD5.ID()})
			}
		}
		for pd, err := range ocfl.Digest(ctx, ocfl.NewFS(fsys), jobs) {
			be.NilErr(t, err)
			valid, err := pd.Validate(ctx, ocfl.NewFS(fsys), ".")
			be.True(t, valid)
			be.NilErr(t, err)
		}
		for pd, err := range ocfl.Digest(ctx, ocfl.NewFS(fsys), jobs) {
			be.NilErr(t, err)
			pd.Digests["sha1"] = "baddigest"
			valid, err := pd.Validate(ctx, ocfl.NewFS(fsys), ".")
			be.False(t, valid)
			var digestErr *ocfl.DigestError
			be.True(t, errors.As(err, &digestErr))
		}
		for pd, err := range ocfl.Digest(ctx, ocfl.NewFS(fsys), jobs) {
			be.NilErr(t, err)
			// fsys with different content
			fsys := fstest.MapFS{pd.Path: &fstest.MapFile{Data: []byte("changed!")}}
			valid, err := pd.Validate(ctx, ocfl.NewFS(fsys), ".")
			be.False(t, valid)
			var digestErr *ocfl.DigestError
			be.True(t, errors.As(err, &digestErr))
		}
		for pd, err := range ocfl.Digest(ctx, ocfl.NewFS(fsys), jobs) {
			be.NilErr(t, err)
			fsys := fstest.MapFS{} // file is missing
			valid, err := pd.Validate(ctx, ocfl.NewFS(fsys), ".")
			be.False(t, valid)
			be.True(t, errors.Is(err, fs.ErrNotExist))
		}
	})
}
