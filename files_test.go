package ocfl_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

func TestFilesBaseStat(t *testing.T) {
	ctx := context.Background()
	testFS := ocfl.NewFS(fstest.MapFS{
		"file.txt":         &fstest.MapFile{Data: []byte("content")},
		"a/file.txt":       &fstest.MapFile{Data: []byte("content")},
		"a/b/file.txt":     &fstest.MapFile{Data: []byte("content")},
		"a/b/c/file.txt":   &fstest.MapFile{Data: []byte("content")},
		"a/b/c/d/file.txt": &fstest.MapFile{Data: []byte("content")},
	})
	t.Run("basic", func(t *testing.T) {
		count := 0
		files := ocfl.FilesSub(testFS, "a", "file.txt", "b/file.txt").Stat(ctx)
		for f, err := range files {
			count++
			be.Nonzero(t, f.FS)
			be.Nonzero(t, f.BaseDir)
			be.Nonzero(t, f.Path)
			be.Nonzero(t, f.Info)
			be.True(t, f.Info.Size() > 0)
			be.NilErr(t, err)
		}
		be.Equal(t, 2, count)
	})
	t.Run("missing", func(t *testing.T) {
		files := ocfl.FilesSub(testFS, ".", "missing").Stat(ctx)
		for _, err := range files {
			be.True(t, errors.Is(err, fs.ErrNotExist))
		}
	})
	t.Run("invalid dir", func(t *testing.T) {
		files := ocfl.FilesSub(testFS, "../", "missing").Stat(ctx)
		for _, err := range files {
			be.True(t, errors.Is(err, fs.ErrInvalid))
		}
	})
	t.Run("invalid name", func(t *testing.T) {
		files := ocfl.FilesSub(testFS, ".", "../missing").Stat(ctx)
		for _, err := range files {
			be.True(t, errors.Is(err, fs.ErrInvalid))
		}
	})
	t.Run("with s3", func(t *testing.T) {
		if !testutil.S3Enabled() {
			t.Log("skipping")
			return
		}
		fsys := testutil.TmpS3FS(t, testFS)
		count := 0
		names := []string{
			"file.txt",
			"b/file.txt",
			"b/c/file.txt",
		}
		files := ocfl.FilesSub(fsys, "a", names...).Stat(ctx)
		for f, err := range files {
			be.NilErr(t, err)
			count++
			be.Nonzero(t, f.FS)
			be.Nonzero(t, f.BaseDir)
			be.Nonzero(t, f.Path)
			be.Nonzero(t, f.Info)
			be.True(t, f.Info.Size() > 0)
		}
		be.Equal(t, len(names), count)
	})
}

func TestWalkFiles(t *testing.T) {
	ctx := context.Background()
	t.Run("basic", func(t *testing.T) {
		fsys := ocfl.NewFS(fstest.MapFS{
			"file.txt": &fstest.MapFile{Data: []byte("content")},
		})
		count := 0
		files, errFn := ocfl.WalkFiles(ctx, fsys, ".")
		for ref := range files {
			be.Equal(t, fsys, ref.FS)
			be.Equal(t, ".", ref.BaseDir)
			be.Equal(t, "file.txt", ref.Path)
			be.Equal(t, len("content"), int(ref.Info.Size()))
			count++
		}
		be.NilErr(t, errFn())
		be.Equal(t, 1, count)
	})

	t.Run("paths relative to dir", func(t *testing.T) {
		fsys := ocfl.NewFS(fstest.MapFS{
			"file.txt":         &fstest.MapFile{Data: []byte("content")},
			"a/file.txt":       &fstest.MapFile{Data: []byte("content")},
			"a/b/file.txt":     &fstest.MapFile{Data: []byte("content")},
			"a/b/c/file.txt":   &fstest.MapFile{Data: []byte("content")},
			"a/b/c/d/file.txt": &fstest.MapFile{Data: []byte("content")},
		})
		var count int
		files, errFn := ocfl.WalkFiles(ctx, fsys, "a")
		for ref := range files {
			be.False(t, strings.HasPrefix(ref.Path, "a/"))
			count++
		}
		be.Equal(t, 4, count)
		be.NilErr(t, errFn())
	})

	t.Run("irregular file type ok", func(t *testing.T) {
		fsys := ocfl.NewFS(fstest.MapFS{
			"file.txt": &fstest.MapFile{
				Data: []byte("content"),
				Mode: fs.ModeIrregular,
			},
		})
		files, errFn := ocfl.WalkFiles(ctx, fsys, ".")
		for range files {
		}
		be.NilErr(t, errFn())
	})

	t.Run("use FileWalker implementation", func(t *testing.T) {
		fsys := &mockFilesFS{}
		files, errFn := ocfl.WalkFiles(ctx, fsys, ".")
		for range files {
		}
		be.NilErr(t, errFn())
	})

	t.Run("empty path is error", func(t *testing.T) {
		fsys := ocfl.NewFS(fstest.MapFS{
			"file.txt": &fstest.MapFile{Data: []byte("content")},
		})
		files, errFn := ocfl.WalkFiles(ctx, fsys, "")
		for range files {
		}
		be.True(t, errors.Is(errFn(), fs.ErrInvalid))
	})
	t.Run("invalid file types", func(t *testing.T) {
		fsys := ocfl.NewFS(fstest.MapFS{
			"symlink": &fstest.MapFile{Mode: fs.ModeSymlink},
			"device":  &fstest.MapFile{Mode: fs.ModeDevice},
			"file":    &fstest.MapFile{Mode: fs.ModeSocket},
			"pipe":    &fstest.MapFile{Mode: fs.ModeNamedPipe},
		})
		files, errFn := ocfl.WalkFiles(ctx, fsys, ".")
		for range files {
		}
		be.True(t, errors.Is(errFn(), ocfl.ErrFileType))
	})

	t.Run("break range", func(t *testing.T) {
		// test that yield ("range function") isn't called again
		// after break
		fsys := ocfl.NewFS(fstest.MapFS{
			"file.txt":         &fstest.MapFile{Data: []byte("content")},
			"a/file.txt":       &fstest.MapFile{Data: []byte("content")},
			"a/b/file.txt":     &fstest.MapFile{Data: []byte("content")},
			"a/b/c/file.txt":   &fstest.MapFile{Data: []byte("content")},
			"a/b/c/d/file.txt": &fstest.MapFile{Data: []byte("content")},
		})
		files, errFn := ocfl.WalkFiles(ctx, fsys, ".")
		for range files {
			break
		}
		be.NilErr(t, errFn())
	})
	t.Run("with s3", func(t *testing.T) {
		if !testutil.S3Enabled() {
			t.Log("skipping")
			return
		}
		testFiles := fstest.MapFS{
			"file.txt":         &fstest.MapFile{Data: []byte("content")},
			"a/file.txt":       &fstest.MapFile{Data: []byte("content")},
			"a/b/file.txt":     &fstest.MapFile{Data: []byte("content")},
			"a/b/c/file.txt":   &fstest.MapFile{Data: []byte("content")},
			"a/b/c/d/file.txt": &fstest.MapFile{Data: []byte("content")},
		}
		fsys := testutil.TmpS3FS(t, ocfl.NewFS(testFiles))
		count := 0
		files, errFn := ocfl.WalkFiles(ctx, fsys, ".")
		for f := range files {
			count++
			be.Nonzero(t, f.FS)
			be.Nonzero(t, f.BaseDir)
			be.Nonzero(t, f.Path)
			be.Nonzero(t, f.Info)
			be.True(t, f.Info.Size() > 0)
		}
		be.NilErr(t, errFn())
		be.Equal(t, len(testFiles), count)
	})
}

func TestDigestFiles(t *testing.T) {
	ctx := context.Background()
	fsys := ocfl.NewFS(fstest.MapFS{
		"a/file.txt": &fstest.MapFile{Data: []byte("content")},
	})
	count := 0
	for fa, err := range ocfl.Files(fsys, "a/file.txt").Digest(ctx, digest.SHA256, digest.SIZE) {
		be.NilErr(t, err)
		be.Equal(t, "a/file.txt", fa.Path)
		be.Equal(t, "sha256", fa.Algorithm.ID())
		be.Nonzero(t, fa.Digests["size"])
		count++
	}
	be.Equal(t, 1, count)
}

type mockFilesFS struct{}

var _ ocfl.FileWalker = (*mockFilesFS)(nil)

func (m *mockFilesFS) OpenFile(_ context.Context, _ string) (fs.File, error) {
	return nil, errors.New("shouldn't be called")
}

func (m *mockFilesFS) ReadDir(_ context.Context, _ string) ([]fs.DirEntry, error) {
	return nil, errors.New("shouldn't be called")
}

func (m *mockFilesFS) WalkFiles(_ context.Context, _ string) (ocfl.FileSeq, func() error) {
	return func(_ func(*ocfl.FileRef) bool) {}, func() error { return nil }
}

func TestValidateFileDigests(t *testing.T) {
	ctx := context.Background()
	testdataFS := ocfl.DirFS(`testdata`)
	t.Run("ok", func(t *testing.T) {
		// build an iter.Seq[*ocfl.FileDigests] from fixture data
		fixtureFiles, walkFn := ocfl.WalkFiles(ctx, testdataFS, "content-fixture")
		fixtureResults := fixtureFiles.Digest(ctx, digest.SHA256, digest.BLAKE2B_256)
		fixtureDigests, digestErr := fixtureResults.UntilErr()
		for err := range fixtureDigests.ValidateBatch(ctx, digest.DefaultRegistry(), 0) {
			be.NilErr(t, err)
		}
		be.NilErr(t, digestErr())
		be.NilErr(t, walkFn())
	})

	t.Run("yields DigestError", func(t *testing.T) {
		fixtureFiles, walkFn := ocfl.WalkFiles(ctx, testdataFS, "content-fixture")
		fixtureResults := fixtureFiles.Digest(ctx, digest.SHA256, digest.SHA1, digest.MD5)
		var makeInvalid ocfl.FileDigestsSeq = func(yield func(*ocfl.FileDigests) bool) {
			for digests, _ := range fixtureResults {
				for id := range digests.Digests {
					digests.Digests[id] = "bad value"
					break // just one bad value
				}
				if !yield(digests) {
					break
				}
			}
		}
		errs := makeInvalid.ValidateBatch(ctx, digest.DefaultRegistry(), 0)
		count := 0
		for err := range errs {
			count++
			var digErr *digest.DigestError
			be.True(t, errors.As(err, &digErr))
		}
		be.Equal(t, 6, count)
		be.NilErr(t, walkFn())
	})
}
