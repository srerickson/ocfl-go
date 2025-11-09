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

func TestFileRef_sValidate(t *testing.T) {
	ctx := context.Background()
	_, fsys, paths := setupTestFS()
	digestedFiles := digestFiles(t, ctx, fsys, paths)
	reg := digest.DefaultRegistry()

	// Find the file ref for "a/file.txt"
	var fileRef *digest.FileRef
	for _, f := range digestedFiles {
		if f.Path == "a/file.txt" {
			fileRef = f
			break
		}
	}
	if fileRef == nil {
		t.Fatal("could not find a/file.txt in digested files")
	}

	t.Run("validates successfully with primary digests", func(t *testing.T) {
		err := fileRef.Validate(ctx, reg)
		be.NilErr(t, err)
	})

	t.Run("validates successfully with fixity digests", func(t *testing.T) {
		err := fileRef.Validate(ctx, reg.Append(digest.SIZE))
		be.NilErr(t, err)
	})

	t.Run("validation fails with wrong primary digest",
		func(t *testing.T) {
			badFileRef := &digest.FileRef{
				FileRef: fileRef.FileRef,
				Digests: digest.Set{"sha256": "wrongdigest"},
			}
			err := badFileRef.Validate(ctx, reg)
			var digestErr *digest.DigestError
			be.True(t, errors.As(err, &digestErr))
			be.Equal(t, digestErr.Path, "a/file.txt")
			be.Equal(t, digestErr.Alg, "sha256")
			be.Equal(t, digestErr.Expected, "wrongdigest")
			be.False(t, digestErr.IsFixity)
		})

	t.Run("validation fails with wrong fixity digest", func(t *testing.T) {
		badFileRef := &digest.FileRef{
			FileRef: fileRef.FileRef,
			Digests: fileRef.Digests,
			Fixity:  digest.Set{"size": "999"},
		}
		err := badFileRef.Validate(ctx, reg.Append(digest.SIZE))
		var digestErr *digest.DigestError
		be.True(t, errors.As(err, &digestErr))
		be.Equal(t, digestErr.Path, "a/file.txt")
		be.Equal(t, digestErr.Alg, "size")
		be.Equal(t, digestErr.Expected, "999")
		be.True(t, digestErr.IsFixity)
	})

	t.Run("validation fails when file doesn't exist", func(t *testing.T) {
		badFileRef := &digest.FileRef{
			FileRef: ocflfs.FileRef{
				FS:      fsys,
				BaseDir: ".",
				Path:    "nonexistent.txt",
			},
			Digests: digest.Set{"sha256": "abc"},
		}
		err := badFileRef.Validate(ctx, reg)
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})

	t.Run("validation fails when content changes", func(t *testing.T) {
		// Create a fresh filesystem and digest the file
		freshData := fstest.MapFS{
			"test.txt": &fstest.MapFile{Data: []byte("content")},
		}
		freshFS := ocflfs.NewWrapFS(freshData)
		inputFiles := ocflfs.Files(freshFS, "test.txt")
		var freshFileRef *digest.FileRef
		for fa, err := range digest.DigestFilesBatch(ctx, inputFiles,
			5, digest.SHA256) {
			be.NilErr(t, err)
			freshFileRef = fa
		}

		// Now change the content
		freshData["test.txt"] = &fstest.MapFile{
			Data: []byte("changed content"),
		}

		err := freshFileRef.Validate(ctx, reg)
		var digestErr *digest.DigestError
		be.True(t, errors.As(err, &digestErr))
		be.Equal(t, digestErr.Path, "test.txt")
	})

	t.Run("allDigests returns error on conflict", func(t *testing.T) {
		// Create FileRef with conflicting primary and fixity values
		conflictRef := &digest.FileRef{
			FileRef: fileRef.FileRef,
			Digests: digest.Set{"sha256": "digest1"},
			Fixity:  digest.Set{"sha256": "digest2"},
		}
		err := conflictRef.Validate(ctx, reg)
		be.Nonzero(t, err)
		be.In(t, "fixity value conflicts with primary", err.Error())
	})
}

func TestSetMethods(t *testing.T) {
	t.Run("Add successfully adds new digests", func(t *testing.T) {
		s1 := digest.Set{"sha256": "abc"}
		s2 := digest.Set{"sha512": "def"}
		err := s1.Add(s2)
		be.NilErr(t, err)
		be.Equal(t, "abc", s1["sha256"])
		be.Equal(t, "def", s1["sha512"])
	})

	t.Run("Add allows same values case-insensitively", func(t *testing.T) {
		s1 := digest.Set{"sha256": "ABC"}
		s2 := digest.Set{"sha256": "abc"}
		err := s1.Add(s2)
		be.NilErr(t, err)
	})

	t.Run("Add returns error on conflict", func(t *testing.T) {
		s1 := digest.Set{"sha256": "abc"}
		s2 := digest.Set{"sha256": "def"}
		err := s1.Add(s2)
		var digestErr *digest.DigestError
		be.True(t, errors.As(err, &digestErr))
		be.Equal(t, "sha256", digestErr.Alg)
		be.Equal(t, "abc", digestErr.Expected)
		be.Equal(t, "def", digestErr.Got)
	})

	t.Run("Algorithms returns nil for empty set", func(t *testing.T) {
		s := digest.Set{}
		algs := s.Algorithms()
		if algs != nil {
			t.Errorf("expected nil, got %v", algs)
		}
	})

	t.Run("Algorithms returns algorithm IDs", func(t *testing.T) {
		s := digest.Set{"sha256": "abc", "sha512": "def"}
		algs := s.Algorithms()
		be.Equal(t, 2, len(algs))
		be.True(t, slices.Contains(algs, "sha256"))
		be.True(t, slices.Contains(algs, "sha512"))
	})
}

func TestDigestError(t *testing.T) {
	t.Run("Error message without path", func(t *testing.T) {
		err := digest.DigestError{
			Alg:      "sha256",
			Got:      "abc",
			Expected: "def",
		}
		be.Equal(t,
			`unexpected sha256 value: "abc", expected="def"`,
			err.Error())
	})

	t.Run("Error message with path", func(t *testing.T) {
		err := digest.DigestError{
			Path:     "file.txt",
			Alg:      "sha256",
			Got:      "abc",
			Expected: "def",
		}
		be.Equal(t,
			`unexpected sha256 for "file.txt": "abc", expected="def"`,
			err.Error())
	})
}

func TestMultiDigesterSum(t *testing.T) {
	t.Run("Sum returns empty string for unknown algorithm",
		func(t *testing.T) {
			dig := digest.DefaultRegistry().NewMultiDigester(
				digest.SHA256.ID())
			dig.Write([]byte("test"))
			be.Equal(t, "", dig.Sum("unknown"))
		})

	t.Run("Sum returns digest for known algorithm", func(t *testing.T) {
		dig := digest.DefaultRegistry().NewMultiDigester(
			digest.SHA256.ID())
		dig.Write([]byte("test"))
		sum := dig.Sum(digest.SHA256.ID())
		be.Nonzero(t, sum)
	})
}

func TestRegistryMethods(t *testing.T) {
	t.Run("All returns all algorithms", func(t *testing.T) {
		reg := digest.DefaultRegistry()
		algs := reg.All()
		be.Nonzero(t, len(algs))
		// Should include standard algorithms
		be.True(t, slices.ContainsFunc(algs, func(a digest.Algorithm) bool {
			return a.ID() == digest.SHA256.ID()
		}))
	})

	t.Run("IDs returns all algorithm IDs", func(t *testing.T) {
		reg := digest.DefaultRegistry()
		ids := reg.IDs()
		be.Nonzero(t, len(ids))
		be.True(t, slices.Contains(ids, digest.SHA256.ID()))
		be.True(t, slices.Contains(ids, digest.SHA512.ID()))
	})

	t.Run("Len returns number of algorithms", func(t *testing.T) {
		reg := digest.DefaultRegistry()
		be.Equal(t, len(reg.All()), reg.Len())
	})

	t.Run("MustGet returns algorithm", func(t *testing.T) {
		reg := digest.DefaultRegistry()
		alg := reg.MustGet(digest.SHA256.ID())
		be.Equal(t, digest.SHA256.ID(), alg.ID())
	})

	t.Run("MustGet panics on unknown algorithm", func(t *testing.T) {
		defer func() {
			r := recover()
			be.Nonzero(t, r)
		}()
		reg := digest.DefaultRegistry()
		reg.MustGet("unknown-algorithm")
	})

	t.Run("NewDigester returns error for unknown algorithm",
		func(t *testing.T) {
			reg := digest.DefaultRegistry()
			_, err := reg.NewDigester("unknown-algorithm")
			be.Nonzero(t, err)
		})
}

func TestDigestFiles(t *testing.T) {
	ctx := context.Background()
	_, fsys, paths := setupTestFS()

	t.Run("digests files with single goroutine", func(t *testing.T) {
		inputFiles := ocflfs.Files(fsys, paths...)
		var count int
		for fa, err := range digest.DigestFiles(ctx, inputFiles,
			digest.SHA256) {
			be.NilErr(t, err)
			be.Nonzero(t, fa.Digests["sha256"])
			count++
		}
		be.Equal(t, 2, count)
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
