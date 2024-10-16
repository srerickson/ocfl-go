package ocfl_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
)

func TestDigestFS(t *testing.T) {
	var testMD5Sums = map[string]string{
		"hello.csv": "9d02fa6e9dd9f38327f7b213daa28be6",
	}
	fsys := ocfl.DirFS(filepath.Join("testdata", "content-fixture"))
	ctx := context.Background()
	t.Run("no input", func(t *testing.T) {
		setup := func(add func(name string, algs []digest.Algorithm) bool) {}
		for range ocfl.Digest(ctx, fsys, setup) {
			t.Error("shouldn't be called")
		}
	})
	t.Run("missing file", func(t *testing.T) {
		setup := func(add func(name string, algs []digest.Algorithm) bool) {
			add(filepath.Join("missingfile"), []digest.Algorithm{digest.MD5})
		}
		for _, err := range ocfl.Digest(ctx, fsys, setup) {
			be.True(t, err != nil)
		}
	})
	t.Run("minimal, one existing file, md5", func(t *testing.T) {
		setup := func(add func(name string, algs []digest.Algorithm) bool) {
			add("hello.csv", []digest.Algorithm{digest.MD5})
		}
		for r, err := range ocfl.Digest(ctx, fsys, setup) {
			be.NilErr(t, err)
			be.Equal(t, testMD5Sums[r.Path], r.Digests[digest.MD5.ID()])
		}
	})
	t.Run("multiple files, md5, sha1", func(t *testing.T) {
		algs := []digest.Algorithm{digest.MD5, digest.SHA1}
		setup := func(add func(name string, algs []digest.Algorithm) bool) {
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
		setup := func(add func(name string, algs []digest.Algorithm) bool) {
			add("hello.csv", nil)
		}
		for r, err := range ocfl.Digest(ctx, fsys, setup) {
			be.NilErr(t, err)
			be.Zero(t, len(r.Digests))
		}
	})
}
