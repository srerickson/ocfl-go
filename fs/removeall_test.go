package fs_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"iter"
	"testing"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// removeWalkEntry is a scripted fs.DirEntry for removeWalkFS.
type removeWalkEntry struct {
	name  string
	isDir bool
}

func (e *removeWalkEntry) Name() string               { return e.name }
func (e *removeWalkEntry) IsDir() bool                { return e.isDir }
func (e *removeWalkEntry) Type() fs.FileMode          { return 0 }
func (e *removeWalkEntry) Info() (fs.FileInfo, error) { return nil, errors.New("not implemented") }

// removeWalkFS is a scripted in-memory FS implementing FS, WriteFS, and
// DirEntriesFS so that fs.RemoveAll can be exercised end to end through its
// generic "." walk. It records every Remove, RemoveAll, and DirEntries call
// (with the exact name argument, in call order) and returns scripted errors:
//
//   - removeErr[name] is the error Remove returns for name (nil when absent);
//   - removeAllErr[name] is the error RemoveAll returns for name. A backend
//     that can empty its root in one batch (S3) returns nil for "."; a backend
//     that must not remove its storage root (local) returns an error for ".".
type removeWalkFS struct {
	entries      map[string][]*removeWalkEntry
	removeErr    map[string]error
	removeAllErr map[string]error

	removeCalls     []string // names passed to Remove, in call order
	removeAllCalls  []string // names passed to RemoveAll, in call order
	dirEntriesCalls []string // names passed to DirEntries, in call order
}

var (
	_ ocflfs.FS           = (*removeWalkFS)(nil)
	_ ocflfs.WriteFS      = (*removeWalkFS)(nil)
	_ ocflfs.DirEntriesFS = (*removeWalkFS)(nil)
)

func (f *removeWalkFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (f *removeWalkFS) DirEntries(ctx context.Context, name string) iter.Seq2[fs.DirEntry, error] {
	f.dirEntriesCalls = append(f.dirEntriesCalls, name)
	scripts := f.entries[name]
	return func(yield func(fs.DirEntry, error) bool) {
		for _, e := range scripts {
			if !yield(e, nil) {
				return
			}
		}
	}
}

func (f *removeWalkFS) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	return 0, errors.New("not implemented")
}

func (f *removeWalkFS) Remove(ctx context.Context, name string) error {
	f.removeCalls = append(f.removeCalls, name)
	return f.removeErr[name]
}

func (f *removeWalkFS) RemoveAll(ctx context.Context, name string) error {
	f.removeAllCalls = append(f.removeAllCalls, name)
	return f.removeAllErr[name]
}

// TestRemoveAll_Dot_PartialFailure pins the partial-failure semantics of
// fs.RemoveAll("."): when one entry fails to remove, the walk must continue
// with the remaining entries and return every per-entry failure joined with
// errors.Join. The old implementation returned on the first error, deleting
// only the first entry and hiding that the root was only partially removed —
// it fails this test because removeCalls stops at the first failing entry and
// errors.Is(err, errC) is false.
func TestRemoveAll_Dot_PartialFailure(t *testing.T) {
	errA := errors.New("remove a.txt failed")
	errC := errors.New("remove c.txt failed")
	errRefuseRoot := errors.New("backend refuses to remove storage root")
	fsys := &removeWalkFS{
		entries: map[string][]*removeWalkEntry{
			".": {{name: "a.txt"}, {name: "b.txt"}, {name: "c.txt"}},
		},
		removeErr:    map[string]error{"a.txt": errA, "c.txt": errC},
		removeAllErr: map[string]error{".": errRefuseRoot},
	}

	err := ocflfs.RemoveAll(context.Background(), fsys, ".")

	// Every entry was attempted, in order, despite a.txt failing first.
	be.DeepEqual(t, []string{"a.txt", "b.txt", "c.txt"}, fsys.removeCalls)

	// The result is the errors.Join of every per-entry failure: both
	// sentinels are found via errors.Is, and the backend's root-refusal
	// error is NOT part of the result (it describes the backend guard, not
	// a failed entry).
	be.Nonzero(t, err)
	be.True(t, errors.Is(err, errA))
	be.True(t, errors.Is(err, errC))
	be.True(t, !errors.Is(err, errRefuseRoot))

	// The backend's own RemoveAll(".") was still probed first, and the
	// fallback walked "." exactly once.
	be.DeepEqual(t, []string{"."}, fsys.removeAllCalls)
	be.DeepEqual(t, []string{"."}, fsys.dirEntriesCalls)
}

// TestRemoveAll_Dot_PrefersBackendBatch pins that fs.RemoveAll(".") delegates
// to the backend's own RemoveAll(".") when the backend can empty the root in
// one batch (e.g. S3: one bucket-wide listing plus batched DeleteObjects),
// instead of deleting every top-level entry one by one. The old
// implementation never called the backend's RemoveAll for "." — it always
// walked — so this test fails before the fix because removeCalls is non-empty
// and dirEntriesCalls is non-empty.
func TestRemoveAll_Dot_PrefersBackendBatch(t *testing.T) {
	fsys := &removeWalkFS{
		entries: map[string][]*removeWalkEntry{
			".": {{name: "a.txt"}, {name: "b.txt"}},
		},
		// removeAllErr has no "." entry: the backend accepts RemoveAll(".")
		// and empties the root itself, batch-style.
	}

	err := ocflfs.RemoveAll(context.Background(), fsys, ".")
	be.NilErr(t, err)

	// The backend's batched RemoveAll(".") was called exactly once...
	be.DeepEqual(t, []string{"."}, fsys.removeAllCalls)
	// ...and the per-entry walk never ran: no Remove calls, no DirEntries.
	be.Zero(t, fsys.removeCalls)
	be.Zero(t, fsys.dirEntriesCalls)
}

// TestRemoveAll_Dot_RecursionPrefixedPaths pins that the "." fallback walk
// threads the accumulated directory prefix through recursion with path.Join
// instead of passing bare entry names. DirEntries yields names relative to
// the listed directory, which may be clean basenames or full relative paths,
// and the walk must hand the backend the joined, normalized full path.
func TestRemoveAll_Dot_RecursionPrefixedPaths(t *testing.T) {
	errRefuseRoot := errors.New("backend refuses to remove storage root")

	t.Run("nested directory recursed via RemoveAll with joined path", func(t *testing.T) {
		// A clean basename "sub" must be dispatched to the backend's
		// RemoveAll with the full relative path path.Join(".", "sub"),
		// not handed to Remove as a bare file delete.
		fsys := &removeWalkFS{
			entries: map[string][]*removeWalkEntry{
				".":   {{name: "sub", isDir: true}, {name: "top.txt"}},
				"sub": {{name: "file.txt"}},
			},
			removeAllErr: map[string]error{".": errRefuseRoot},
		}

		err := ocflfs.RemoveAll(context.Background(), fsys, ".")
		be.NilErr(t, err)

		// Probe of the backend's RemoveAll("."), then the recursion into
		// the subdirectory with the joined path.
		be.DeepEqual(t, []string{".", "sub"}, fsys.removeAllCalls)
		be.DeepEqual(t, []string{"top.txt"}, fsys.removeCalls)
	})

	t.Run("entry names need not be top-level basenames", func(t *testing.T) {
		// DirEntries(".") is allowed to yield full relative paths (an
		// S3-style flattened listing does), which may carry redundant
		// elements. The walk must normalize them with path.Join: a dir
		// entry named "./sub" must reach the backend as "sub", never as
		// the bare "./sub". The old implementation passed entry.Name()
		// verbatim, so this test fails before the fix.
		fsys := &removeWalkFS{
			entries: map[string][]*removeWalkEntry{
				".": {{name: "./sub", isDir: true}, {name: "top.txt"}},
			},
			removeAllErr: map[string]error{".": errRefuseRoot},
		}

		err := ocflfs.RemoveAll(context.Background(), fsys, ".")
		be.NilErr(t, err)

		be.DeepEqual(t, []string{".", "sub"}, fsys.removeAllCalls)
		be.DeepEqual(t, []string{"top.txt"}, fsys.removeCalls)
	})
}

// TestRemoveAll_NonDotDelegatesToBackend pins that the "." special case does
// not leak into ordinary RemoveAll: any other name is passed straight to the
// backend's RemoveAll, and the generic walk is never triggered.
func TestRemoveAll_NonDotDelegatesToBackend(t *testing.T) {
	fsys := &removeWalkFS{
		entries: map[string][]*removeWalkEntry{
			"dir": {{name: "file.txt"}},
		},
	}

	err := ocflfs.RemoveAll(context.Background(), fsys, "dir")
	be.NilErr(t, err)

	be.DeepEqual(t, []string{"dir"}, fsys.removeAllCalls)
	be.Zero(t, fsys.removeCalls)
	be.Zero(t, fsys.dirEntriesCalls)
}
