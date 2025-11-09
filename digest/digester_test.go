package digest_test

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
	"maps"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"golang.org/x/crypto/blake2b"
)

func TestDigester(t *testing.T) {
	testDigester := func(t *testing.T, algs []string) {
		t.Helper()
		data := []byte("content")
		algRegistry := digest.DefaultRegistry()
		dig := algRegistry.NewMultiDigester(algs...)
		if _, err := io.Copy(dig, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		result := dig.Sums()
		if l := len(result); l != len(algs) {
			t.Fatalf("Sums() returned wrong number of entries: got=%d, expect=%d", l, len(algs))
		}
		expect := digest.Set{}
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
		if err := digest.Validate(bytes.NewReader(data), result, algRegistry); err != nil {
			t.Error("Validate() unexpected error:", err)
		}
		// add invalid entry
		result[digest.SHA1.ID()] = "invalid"
		err := digest.Validate(bytes.NewReader(data), result, algRegistry)
		if err == nil {
			t.Error("Validate() didn't return an error for an invalid DigestSet")
		}
		var digestErr *digest.DigestError
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
	t.Run("no algs", func(t *testing.T) {
		d := digest.DefaultRegistry().NewMultiDigester()
		_, err := d.Write([]byte("test"))
		be.NilErr(t, err)
		be.Zero(t, len(d.Sums()))
	})
	t.Run("1 alg", func(t *testing.T) {
		testDigester(t, []string{digest.MD5.ID()})
	})
	t.Run("2 algs", func(t *testing.T) {
		testDigester(t, []string{digest.MD5.ID(), digest.BLAKE2B.ID()})
	})
}

// setupTestFS creates a test filesystem with sample files
func setupTestFS() (fstest.MapFS, ocflfs.FS, []string) {
	testData := fstest.MapFS{
		"a/file.txt": &fstest.MapFile{Data: []byte("content")},
		"mydata.csv": &fstest.MapFile{Data: []byte("content,1,2,3")},
	}
	fsys := ocflfs.NewWrapFS(testData)
	paths := slices.Collect(maps.Keys(testData))
	return testData, fsys, paths
}

func TestDigestFilesBatch(t *testing.T) {
	ctx := context.Background()
	testData, fsys, paths := setupTestFS()

	t.Run("breaking early doesn't panic", func(t *testing.T) {
		inputFiles := ocflfs.Files(fsys, paths...)
		for range digest.DigestFilesBatch(ctx, inputFiles, 5,
			digest.SHA256, digest.SIZE) {
			break
		}
	})

	t.Run("digests files successfully", func(t *testing.T) {
		inputFiles := ocflfs.Files(fsys, paths...)
		var digestedFiles []*digest.FileRef
		for fa, err := range digest.DigestFilesBatch(ctx, inputFiles, 5,
			digest.SHA256, digest.SIZE) {
			be.NilErr(t, err)
			be.Nonzero(t, fa.Path)
			be.Nonzero(t, fa.Digests["sha256"])
			be.Nonzero(t, fa.Fixity["size"])
			digestedFiles = append(digestedFiles, fa)
		}
		be.Equal(t, len(digestedFiles), len(testData))
	})
}

func TestValidateFilesBatch(t *testing.T) {
	ctx := context.Background()
	testData, fsys, paths := setupTestFS()
	digestedFiles := digestFiles(t, ctx, fsys, paths)
	reg := digest.DefaultRegistry()

	t.Run("validates with default registry", func(t *testing.T) {
		for err := range digest.ValidateFilesBatch(ctx,
			slices.Values(digestedFiles), reg, 0) {
			t.Error(err)
		}
	})

	t.Run("validates with size algorithm", func(t *testing.T) {
		for err := range digest.ValidateFilesBatch(ctx,
			slices.Values(digestedFiles), reg.Append(digest.SIZE), 0) {
			t.Error(err)
		}
	})

	t.Run("validation fails for invalid digests and missing files",
		func(t *testing.T) {
			invalidDigests := []*digest.FileRef{
				{
					FileRef: digestedFiles[0].FileRef,
					// wrong sha256
					Digests: digest.Set{"sha256": "abc"},
					Fixity:  digestedFiles[0].Fixity,
				},
				{
					FileRef: digestedFiles[1].FileRef,
					Digests: digestedFiles[1].Digests,
					// wrong size
					Fixity: digest.Set{"size": "2000"},
				},
				{
					// missing file
					FileRef: ocflfs.FileRef{
						FS:      fsys,
						BaseDir: ".",
						Path:    "missing",
					},
				},
			}
			errCount := 0
			for err := range digest.ValidateFilesBatch(ctx,
				slices.Values(invalidDigests), reg.Append(digest.SIZE), 0) {
				var digestErr *digest.DigestError
				var pathErr *fs.PathError
				switch {
				case errors.As(err, &digestErr) &&
					digestErr.Path == "a/file.txt":
				case errors.As(err, &digestErr) &&
					digestErr.Path == "mydata.csv":
				case errors.As(err, &pathErr) && pathErr.Path == "missing":
					be.True(t, errors.Is(err, fs.ErrNotExist))
				default:
					t.Error("unexpected error", err)
				}
				errCount++
			}
			be.Equal(t, 3, errCount)
		})

	t.Run("validation fails when content changes", func(t *testing.T) {
		testData["a/file.txt"] = &fstest.MapFile{
			Data: []byte("new content"),
		}
		errCount := 0
		for err := range digest.ValidateFilesBatch(ctx,
			slices.Values(digestedFiles), reg, 0) {
			var digestErr *digest.DigestError
			be.True(t, errors.As(err, &digestErr))
			be.Equal(t, "a/file.txt", digestErr.Path)
			errCount++
		}
		be.Equal(t, 1, errCount)
	})

	t.Run("breaking early doesn't panic", func(t *testing.T) {
		for range digest.ValidateFilesBatch(ctx,
			slices.Values(digestedFiles), reg, 0) {
			break
		}
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
	d, err := digest.DefaultRegistry().NewDigester(algID)
	if err != nil {
		return "", err
	}
	d.Write(val)
	h.Write(val)
	exp := hex.EncodeToString(h.Sum(nil))
	got := d.String()
	if exp != got {
		return exp, fmt.Errorf("%s value: got=%q, expected=%q", algID, got, exp)
	}
	return exp, nil
}

// digestFiles digests a set of files and returns the results
func digestFiles(t *testing.T, ctx context.Context, fsys ocflfs.FS,
	paths []string) []*digest.FileRef {
	t.Helper()
	inputFiles := ocflfs.Files(fsys, paths...)
	var digestedFiles []*digest.FileRef
	for fa, err := range digest.DigestFilesBatch(ctx, inputFiles, 5,
		digest.SHA256, digest.SIZE) {
		be.NilErr(t, err)
		digestedFiles = append(digestedFiles, fa)
	}
	return digestedFiles
}
