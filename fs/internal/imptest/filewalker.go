package imptest

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"iter"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// WalkFiles configures [TestWalkFiles] with the implementation-specific part
// of the walk: the fixture that makes a walk fail.
//
// The behaviors the suite pins are the ones every implementation must share:
//   - early termination: when the yield callback returns false, iteration
//     stops and no further paths are yielded;
//   - error propagation: a walk failure is yielded to the caller as the error
//     element of the (file, error) pair, and after that error yield the
//     iterator yields nothing further;
//   - every yielded *[ocflfs.FileRef] has its FS field set to the backend's
//     own instance, so callers can open the file through the ref without
//     carrying the backend around.
type WalkFiles struct {
	// ErrWalk returns a backend filesystem whose walk of the fixed name
	// "blocked" is guaranteed to fail. The error-propagation subtest uses it
	// to observe how an implementation delivers a walk failure. For s3 this
	// is a BucketFS over an API that errors when asked to list the
	// "blocked/" prefix; for local it is an FS where "blocked" is a regular
	// file where the walk expects a directory, so the directory read fails.
	//
	// The subtest that uses it is skipped pending #165, so the fixture is
	// built and not run; it stays wired up so closing that issue is a
	// one-line change.
	ErrWalk func(t *testing.T) ocflfs.WriteFS
}

// TestWalkFiles asserts the WalkFiles behavior every implementation must
// share, against whichever backend fsys implements. Call it from a backend's
// own external test package.
//
// fsys is seeded with the files a.txt and sub/{b.txt,c.txt} — a flat file plus
// a nested directory. WalkFiles is exercised through the package-level
// [ocflfs.WalkFiles] entry point, which dispatches to the backend's own
// optimized WalkFiles when it implements [ocflfs.FileWalker] (s3 does; local
// walks via DirEntries), so the suite covers the same code path a library
// caller uses for either backend.
//
// The seed enumerates identically on both backends — lexicographic key order
// on s3, sorted DirEntries order on local — so the suite can pin exact paths,
// ordering, and relative Path values without backend-specific logic.
func TestWalkFiles(t *testing.T, fsys ocflfs.WriteFS, opts WalkFiles) {
	t.Helper()
	ctx := context.Background()

	// A walk yield: the FileRef and error delivered together, in order.
	type walkYield struct {
		ref *ocflfs.FileRef
		err error
	}

	seed := []struct{ name, content string }{
		{"a.txt", "content-a"},
		{"sub/b.txt", "content-b"},
		{"sub/c.txt", "content-c"},
	}
	for _, file := range seed {
		if _, err := fsys.Write(ctx, file.name, strings.NewReader(file.content)); err != nil {
			t.Fatalf("seeding %q: %v", file.name, err)
		}
	}

	// walkAll pulls every yield out of WalkFiles(fsys, dir) and returns them
	// in order, so subtests can assert on the full sequence.
	walkAll := func(fsys ocflfs.FS, dir string) []walkYield {
		t.Helper()
		var got []walkYield
		for ref, err := range ocflfs.WalkFiles(ctx, fsys, dir) {
			got = append(got, walkYield{ref: ref, err: err})
		}
		return got
	}

	t.Run("every yield carries the backend FS and a complete FileRef", func(t *testing.T) {
		got := walkAll(fsys, ".")
		be.Equal(t, len(seed), len(got))

		// Both backends enumerate the seed in the same order: the flat file
		// first (a.txt < sub), then the nested files depth-first.
		for i, y := range got {
			be.NilErr(t, y.err)
			be.Nonzero(t, y.ref)
			be.Equal(t, seed[i].name, y.ref.FullPath())
			be.Equal(t, ".", y.ref.BaseDir)
			// The ref must be complete: metadata populated, and the FS field
			// pointing at the backend instance we walked, so opening the file
			// through the ref reaches the source bytes.
			be.Nonzero(t, y.ref.Info)
			be.True(t, y.ref.FS == fsys)
			file, err := y.ref.Open(ctx)
			be.NilErr(t, err)
			data, err := io.ReadAll(file)
			be.NilErr(t, err)
			be.NilErr(t, file.Close())
			be.Equal(t, seed[i].content, string(data))
		}
	})

	t.Run("early termination when the callback returns false", func(t *testing.T) {
		// Pull the iterator by hand so we control the callback exactly:
		// stop() makes the yield function return false mid-iteration, and a
		// subsequent pull must report end-of-iteration instead of yielding
		// further paths.
		next, stop := iter.Pull2(ocflfs.WalkFiles(ctx, fsys, "."))
		defer stop()

		ref, err, ok := next()
		be.True(t, ok)
		be.NilErr(t, err)
		be.Nonzero(t, ref)
		be.Equal(t, "a.txt", ref.FullPath())
		be.True(t, ref.FS == fsys)

		// Callback returns false: iteration must stop. The first pull after
		// stop() finds the iterator already terminated — the remaining seeded
		// paths (sub/b.txt, sub/c.txt) must never be yielded.
		stop()
		ref, err, ok = next()
		be.False(t, ok)
		be.Zero(t, ref)
		be.Zero(t, err)
	})

	t.Run("walk errors are delivered and terminate iteration", func(t *testing.T) {
		// TODO(#165): fileWalk yields the directory-read error and then falls
		// through to e.Name() on the nil entry it was yielded with, so the
		// local walk panics instead of terminating. The s3 backend already
		// satisfies this; drop the skip when #165 fixes the nil deref.
		t.Skip("fileWalk panics on an error-yielding DirEntries; see #165")
		// The fixture's walk of "blocked" fails: s3's ListObjectsV2 errors on
		// that prefix, local's directory read of a regular file fails.
		got := walkAll(opts.ErrWalk(t), "blocked")

		// Exactly one error is yielded and nothing else: the error is
		// returned to the caller as the pair's error element, and no further
		// yields (refs or errors) follow it.
		be.Equal(t, 1, len(got))
		be.Zero(t, got[0].ref)
		be.Nonzero(t, got[0].err)

		// Every backend surfaces walk failures as an *fs.PathError naming the
		// walked path, so callers can handle them uniformly.
		var pathErr *fs.PathError
		be.True(t, errors.As(got[0].err, &pathErr))
		be.Equal(t, "blocked", pathErr.Path)
	})
}
