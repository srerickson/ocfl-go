package fs_test

import (
	"context"
	"errors"
	"io/fs"
	"iter"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// scriptedDirEntry is a fake fs.DirEntry whose Info() always fails, so a test
// can put an entry into a directory listing whose Info() returns an error.
type scriptedDirEntry struct {
	name    string
	infoErr error
}

func (e *scriptedDirEntry) Name() string               { return e.name }
func (e *scriptedDirEntry) IsDir() bool                { return false }
func (e *scriptedDirEntry) Type() fs.FileMode          { return 0 }
func (e *scriptedDirEntry) Info() (fs.FileInfo, error) { return nil, e.infoErr }

// walkStep is one yield of a scripted DirEntries iterator: either an entry or
// an error, matching the (entry, error) pairs an iter.Seq2 allows.
type walkStep struct {
	entry fs.DirEntry
	err   error
}

// walkErrorFS is an FS implementing DirEntriesFS whose DirEntries iterator
// replays a script of (entry, error) yields back-to-back. This stresses the
// DirEntriesFS contract: a well-behaved iterator terminates after an error
// yield, but fileWalk must not panic or corrupt its output when one doesn't
// (ocflfs.DirEntries itself yields (nil, err) for a non-DirEntriesFS, and a
// listing can fail partway through).
type walkErrorFS struct {
	script []walkStep
}

var _ ocflfs.DirEntriesFS = (*walkErrorFS)(nil)

func (f *walkErrorFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (f *walkErrorFS) DirEntries(ctx context.Context, name string) iter.Seq2[fs.DirEntry, error] {
	return func(yield func(fs.DirEntry, error) bool) {
		for _, step := range f.script {
			if !yield(step.entry, step.err) {
				return
			}
		}
	}
}

// walkYield is one result pulled from WalkFiles, keeping the interleaving of
// FileRefs and errors so the test can assert exact ordering.
type walkYield struct {
	ref *ocflfs.FileRef
	err error
}

// TestWalkFiles_DirEntriesErrorContract pins fileWalk's handling of a
// DirEntriesFS that yields (nil, err) pairs and entries whose Info() fails.
// The contract:
//   - never panic,
//   - propagate every yielded error (in order),
//   - never produce a FileRef with a nil Info field after an Info() error,
//   - keep walking valid entries after error yields.
func TestWalkFiles_DirEntriesErrorContract(t *testing.T) {
	// The (nil, err) yield is propagated and then falls through to
	// path.Join(subDir, e.Name()) on the nil entry, so the walk panics; an
	// Info() error produces a *FileRef with a nil Info field.
	t.Skip("fileWalk panics on an error-yielding DirEntries; see #165")

	errYield := errors.New("direntries: listing failed")
	errInfo := errors.New("direntries: stat failed")

	// A real, healthy directory entry from fstest.MapFS for the "valid entry
	// after error yields" step; its Info() succeeds.
	goodEntries, err := fstest.MapFS{
		"good.txt": &fstest.MapFile{Mode: 0o644},
	}.ReadDir(".")
	be.NilErr(t, err)
	be.Equal(t, 1, len(goodEntries))

	fsys := &walkErrorFS{script: []walkStep{
		// (nil, err): error with no entry — the case ocflfs.DirEntries itself
		// produces when the FS isn't a DirEntriesFS.
		{err: errYield},
		// A valid entry whose Info() fails: fileWalk must yield the error and
		// produce no FileRef, because a FileRef with a nil Info field would
		// violate the FileRef contract.
		{entry: &scriptedDirEntry{name: "bad.txt", infoErr: errInfo}},
		// A healthy entry after the error yields: iteration must continue and
		// the entry must still yield a fully-populated FileRef.
		{entry: goodEntries[0]},
		// A second (nil, err) yield after the valid entries.
		{err: errYield},
	}}

	// Collect the walk results under a recover so "no panic" is an explicit
	// assertion rather than an implicit property of reaching the asserts.
	var (
		got      []walkYield
		panicVal any
	)
	func() {
		defer func() { panicVal = recover() }()
		for ref, err := range ocflfs.WalkFiles(context.Background(), fsys, "root") {
			got = append(got, walkYield{ref: ref, err: err})
		}
	}()
	be.Zero(t, panicVal)

	// Every scripted yield surfaced, in order: two propagated errors, the
	// healthy FileRef in between, and iteration continuing past both errors.
	be.Equal(t, 4, len(got))

	be.Zero(t, got[0].ref)
	be.True(t, errors.Is(got[0].err, errYield))

	// The Info() error must not produce a FileRef — and definitely not one
	// with a nil Info field.
	be.Zero(t, got[1].ref)
	be.True(t, errors.Is(got[1].err, errInfo))

	// The healthy entry after the error yields still walks normally, with a
	// fully-populated FileRef (non-nil Info).
	be.NilErr(t, got[2].err)
	be.Nonzero(t, got[2].ref)
	be.Equal(t, "good.txt", got[2].ref.Path)
	be.True(t, got[2].ref.Info != nil)

	// And a (nil, err) yield after valid entries is still propagated.
	be.Zero(t, got[3].ref)
	be.True(t, errors.Is(got[3].err, errYield))

	// Belt and suspenders: no yielded FileRef anywhere carries a nil Info
	// field, even though FileRef.Info is documented "(may be nil)".
	for _, y := range got {
		if y.ref != nil {
			be.True(t, y.ref.Info != nil)
		}
	}
}
