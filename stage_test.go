package ocfl_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

func TestStageFiles(t *testing.T) {
	ctx := context.Background()
	testdataFS := ocflfs.DirFS(`testdata`)
	fixtureFiles := []string{
		"content-fixture/hello.csv",
		"content-fixture/folder1/file.txt",
	}
	files := ocflfs.Files(testdataFS, fixtureFiles...)
	stage, err := ocfl.StageFiles(ctx, files, digest.SHA256, digest.BLAKE2B_160, digest.SIZE)
	be.NilErr(t, err)
	be.Equal(t, stage.DigestAlgorithm.ID(), `sha256`)
	for _, n := range fixtureFiles {
		digest := stage.State.GetDigest(n)
		be.Nonzero(t, digest)
		fsys, path := stage.GetContent(digest)
		be.Nonzero(t, fsys)
		be.Nonzero(t, path)
		be.Nonzero(t, stage.FixitySource.GetFixity(digest)[`size`])
		be.Nonzero(t, stage.FixitySource.GetFixity(digest)[`blake2b-160`])
	}
	t.Run("missing file", func(t *testing.T) {
		files := ocflfs.Files(testdataFS, "missing")
		_, err := ocfl.StageFiles(ctx, files, digest.SHA256, digest.BLAKE2B, digest.SIZE)
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})
}

func TestStageDir(t *testing.T) {
	ctx := context.Background()
	testdataFS := ocflfs.DirFS(`testdata`)
	stage, err := ocfl.StageDir(ctx, testdataFS, "content-fixture", digest.SHA256, digest.MD5)
	be.NilErr(t, err)
	be.Equal(t, `sha256`, stage.DigestAlgorithm.ID())
	be.Equal(t, 3, len(stage.State))
	expectedPath := "folder1/folder2/sculpture-stone-face-head-888027.jpg"
	expDigest := stage.State.GetDigest(expectedPath)
	be.Nonzero(t, expDigest)
	resolveFS, resolvePath := stage.GetContent(expDigest)
	be.Nonzero(t, resolveFS)
	be.Nonzero(t, resolvePath)
	_, err = ocflfs.StatFile(ctx, resolveFS, resolvePath)
	be.NilErr(t, err)
	be.Nonzero(t, stage.FixitySource.GetFixity(expDigest)[`md5`])
}
