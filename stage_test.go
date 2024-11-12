package ocfl_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
)

func TestNewStage(t *testing.T) {
	ctx := context.Background()
	testdataFS := ocfl.DirFS(`testdata`)
	fixtureFiles := []string{
		"content-fixture/hello.csv",
		"content-fixture/folder1/file.txt",
	}
	stage, err := ocfl.Files(testdataFS, fixtureFiles...).
		Digest(ctx, digest.SHA256, digest.BLAKE2B_160, digest.SIZE).
		Stage()
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
		digests, digestErrFn := ocfl.Files(testdataFS, "missing").
			Digest(ctx, digest.SHA256, digest.BLAKE2B_160, digest.SIZE).
			UntilErr()
		_, err := digests.Stage()
		be.NilErr(t, err)
		be.True(t, errors.Is(digestErrFn(), fs.ErrNotExist))
	})
	t.Run("inconsistent digest algorithms", func(t *testing.T) {
		var digests ocfl.FileDigestsSeq = func(yield func(*ocfl.FileDigests) bool) {
			// sha256
			for d := range ocfl.Files(testdataFS, fixtureFiles[0]).Digest(ctx, digest.SHA256) {
				if !yield(d) {
					break
				}
			}
			// sha512
			for d := range ocfl.Files(testdataFS, fixtureFiles[1]).Digest(ctx, digest.SHA512) {
				if !yield(d) {
					break
				}
			}
		}
		_, err := digests.Stage()
		be.In(t, "inconsistent digest algorithms", err.Error())
	})
	t.Run("inconsistent base dir", func(t *testing.T) {
		var digests ocfl.FileDigestsSeq = func(yield func(*ocfl.FileDigests) bool) {
			files, walkErr := ocfl.WalkFiles(ctx, testdataFS, "content-fixture/folder1")
			for d := range files.Digest(ctx, digest.SHA512) {
				if !yield(d) {
					break
				}
			}
			be.NilErr(t, walkErr())
			for d := range ocfl.Files(testdataFS, fixtureFiles[1]).Digest(ctx, digest.SHA512) {
				if !yield(d) {
					break
				}
			}
		}
		_, err := digests.Stage()
		be.In(t, "inconsistent base directory", err.Error())
	})
	t.Run("missing digest", func(t *testing.T) {
		var digests ocfl.FileDigestsSeq = func(yield func(*ocfl.FileDigests) bool) {
			files, walkErr := ocfl.WalkFiles(ctx, testdataFS, "content-fixture/folder1")
			for d := range files.Digest(ctx, digest.SHA512) {
				d.Digests.Delete(digest.SHA512.ID())
				if !yield(d) {
					return
				}
			}
			be.NilErr(t, walkErr())
		}
		_, err := digests.Stage()
		be.In(t, "missing sha512", err.Error())
	})
}

func TestStageDir(t *testing.T) {
	ctx := context.Background()
	testdataFS := ocfl.DirFS(`testdata`)
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
	_, err = ocfl.StatFile(ctx, resolveFS, resolvePath)
	be.NilErr(t, err)
	be.Nonzero(t, stage.FixitySource.GetFixity(expDigest)[`md5`])
}
