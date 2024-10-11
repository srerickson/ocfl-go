package ocfl_test

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

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
		jobs := func(yield func(string, []digest.Algorithm) bool) {
			for name := range fsys {
				yield(name, []digest.Algorithm{digest.SHA256, digest.MD5})
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
			var digestErr *digest.DigestError
			be.True(t, errors.As(err, &digestErr))
		}
		for pd, err := range ocfl.Digest(ctx, ocfl.NewFS(fsys), jobs) {
			be.NilErr(t, err)
			// fsys with different content
			fsys := fstest.MapFS{pd.Path: &fstest.MapFile{Data: []byte("changed!")}}
			valid, err := pd.Validate(ctx, ocfl.NewFS(fsys), ".")
			be.False(t, valid)
			var digestErr *digest.DigestError
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
