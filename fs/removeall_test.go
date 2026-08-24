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
//   - removeAllErr[name] is the error RemoveAll returns for name.
//
// removeWalkFS deliberately does NOT implement ocflfs.RootRemover, so
// fs.RemoveAll(".") takes the generic per-entry fallback against it. See
// rootRemoverFS for the opt-in case.
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

// rootRemoverFS is a removeWalkFS that additionally implements
// ocflfs.RootRemover, standing in for a backend that can empty its own root
// in one bulk operation (the S3 backend's bucket-wide batched delete).
type rootRemoverFS struct {
	*removeWalkFS
	removeRootErr  error
	removeRootCall int
}

var _ ocflfs.RootRemover = (*rootRemoverFS)(nil)

func (f *rootRemoverFS) RemoveRoot(ctx context.Context) error {
	f.removeRootCall++
	return f.removeRootErr
}

// TestRemoveAll_Dot_PartialFailure pins the partial-failure semantics of
// fs.RemoveAll("."): when one entry fails to remove, the walk must continue
// with the remaining entries and return every per-entry failure joined with
// errors.Join. Returning on the first error would delete only the entries up
// to it and hide that the root was left partially removed, so the assertions
// check both halves: removeCalls covers every entry, and every per-entry
// error is reachable with errors.Is.
func TestRemoveAll_Dot_PartialFailure(t *testing.T) {
	errA := errors.New("remove a.txt failed")
	errC := errors.New("remove c.txt failed")
	fsys := &removeWalkFS{
		entries: map[string][]*removeWalkEntry{
			".": {{name: "a.txt"}, {name: "b.txt"}, {name: "c.txt"}},
		},
		removeErr: map[string]error{"a.txt": errA, "c.txt": errC},
	}

	err := ocflfs.RemoveAll(context.Background(), fsys, ".")

	// Every entry was attempted, in order, despite a.txt failing first.
	be.DeepEqual(t, []string{"a.txt", "b.txt", "c.txt"}, fsys.removeCalls)

	// The result is the errors.Join of every per-entry failure: both
	// sentinels are found via errors.Is.
	be.Nonzero(t, err)
	be.True(t, errors.Is(err, errA))
	be.True(t, errors.Is(err, errC))

	// The backend does not implement RootRemover, so its own RemoveAll was
	// never called for "." and the fallback walked "." exactly once.
	be.Zero(t, fsys.removeAllCalls)
	be.DeepEqual(t, []string{"."}, fsys.dirEntriesCalls)
}

// TestRemoveAll_Dot_UsesRootRemover pins that fs.RemoveAll(".") dispatches to
// a backend's RemoveRoot when it implements ocflfs.RootRemover, instead of
// deleting every top-level entry one by one. This is what lets the S3 backend
// empty a bucket with one listing and batched DeleteObjects.
func TestRemoveAll_Dot_UsesRootRemover(t *testing.T) {
	fsys := &rootRemoverFS{
		removeWalkFS: &removeWalkFS{
			entries: map[string][]*removeWalkEntry{
				".": {{name: "a.txt"}, {name: "b.txt"}},
			},
		},
	}

	err := ocflfs.RemoveAll(context.Background(), fsys, ".")
	be.NilErr(t, err)

	// RemoveRoot was called exactly once...
	be.Equal(t, 1, fsys.removeRootCall)
	// ...and the per-entry walk never ran: no RemoveAll, Remove, or
	// DirEntries calls.
	be.Zero(t, fsys.removeAllCalls)
	be.Zero(t, fsys.removeCalls)
	be.Zero(t, fsys.dirEntriesCalls)
}

// TestRemoveAll_Dot_RootRemoverErrorPropagates pins that a RemoveRoot failure
// is returned to the caller unchanged, with no per-entry fallback.
//
// A backend that implements RootRemover owns the operation, and its errors
// are not "refusals" to be retried a different way: a bulk delete that fails
// partway through a large bucket is indistinguishable from a backend guard,
// so retrying with the per-entry walk would hide a real failure behind a
// slower path (and, on S3, cost two requests per surviving object because
// Remove HEAD-checks existence first).
func TestRemoveAll_Dot_RootRemoverErrorPropagates(t *testing.T) {
	errBulk := errors.New("batch delete failed after 2 pages")
	fsys := &rootRemoverFS{
		removeWalkFS: &removeWalkFS{
			entries: map[string][]*removeWalkEntry{
				".": {{name: "a.txt"}, {name: "b.txt"}},
			},
		},
		removeRootErr: errBulk,
	}

	err := ocflfs.RemoveAll(context.Background(), fsys, ".")

	// The backend's error reaches the caller, unwrapped and unjoined.
	be.True(t, errors.Is(err, errBulk))
	be.Equal(t, 1, fsys.removeRootCall)

	// No fallback walk was attempted after the failure.
	be.Zero(t, fsys.removeCalls)
	be.Zero(t, fsys.dirEntriesCalls)
	be.Zero(t, fsys.removeAllCalls)
}

// TestRemoveAll_Dot_RecursionPrefixedPaths pins that the "." fallback walk
// threads the accumulated directory prefix through recursion with path.Join
// instead of passing bare entry names. DirEntries yields names relative to
// the listed directory, which may be clean basenames or full relative paths,
// and the walk must hand the backend the joined, normalized full path.
func TestRemoveAll_Dot_RecursionPrefixedPaths(t *testing.T) {
	t.Run("nested directory recursed via RemoveAll with joined path", func(t *testing.T) {
		// A clean basename "sub" must be dispatched to the backend's
		// RemoveAll with the full relative path path.Join(".", "sub"),
		// not handed to Remove as a bare file delete.
		fsys := &removeWalkFS{
			entries: map[string][]*removeWalkEntry{
				".":   {{name: "sub", isDir: true}, {name: "top.txt"}},
				"sub": {{name: "file.txt"}},
			},
		}

		err := ocflfs.RemoveAll(context.Background(), fsys, ".")
		be.NilErr(t, err)

		// The recursion into the subdirectory uses the joined path.
		be.DeepEqual(t, []string{"sub"}, fsys.removeAllCalls)
		be.DeepEqual(t, []string{"top.txt"}, fsys.removeCalls)
	})

	t.Run("entry names need not be top-level basenames", func(t *testing.T) {
		// DirEntries(".") is allowed to yield full relative paths (an
		// S3-style flattened listing does), which may carry redundant
		// elements. The walk must normalize them with path.Join: a dir
		// entry named "./sub" must reach the backend as "sub", never as
		// the bare "./sub" that entry.Name() reports.
		fsys := &removeWalkFS{
			entries: map[string][]*removeWalkEntry{
				".": {{name: "./sub", isDir: true}, {name: "top.txt"}},
			},
		}

		err := ocflfs.RemoveAll(context.Background(), fsys, ".")
		be.NilErr(t, err)

		be.DeepEqual(t, []string{"sub"}, fsys.removeAllCalls)
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
