package fs_test

// Tests for fs.go: the package-level helpers that dispatch across the
// optional FS interfaces -- Copy over CopyFS/SameBackend, RemoveAll over
// RootRemover, and WalkFiles over DirEntriesFS. Each test drives the
// dispatch with a fake FS that implements exactly the interface subset the
// case is about, so what the helper does and does not require is visible in
// the fake's method set.
//
// Backend behavior behind these helpers is tested in fs/local and fs/s3,
// against the shared contracts in internal/testutil.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"iter"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// spyFS is an in-memory FS that implements FS, WriteFS, CopyFS, and
// SameBackend. It records Copy and Write calls so tests can observe which
// path fs.Copy took, and SameBackend reports the configured value.
type spyFS struct {
	fsys       fstest.MapFS
	same       bool
	copyErr    error
	copyCalls  []string // "src -> dst" in call order
	writeCalls []string // dst names in call order
}

var (
	_ ocflfs.FS          = (*spyFS)(nil)
	_ ocflfs.WriteFS     = (*spyFS)(nil)
	_ ocflfs.CopyFS      = (*spyFS)(nil)
	_ ocflfs.SameBackend = (*spyFS)(nil)
)

func (f *spyFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	return f.fsys.Open(name)
}

func (f *spyFS) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	f.writeCalls = append(f.writeCalls, name)
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	f.fsys[name] = &fstest.MapFile{Data: data}
	return int64(len(data)), nil
}

func (f *spyFS) Remove(ctx context.Context, name string) error {
	delete(f.fsys, name)
	return nil
}

func (f *spyFS) RemoveAll(ctx context.Context, name string) error {
	return nil
}

func (f *spyFS) Copy(ctx context.Context, dst string, src string) (int64, error) {
	f.copyCalls = append(f.copyCalls, src+" -> "+dst)
	return 0, f.copyErr
}

func (f *spyFS) SameBackend(other ocflfs.FS) bool { return f.same }

// basicCopyFS implements FS, WriteFS, and CopyFS but deliberately NOT
// SameBackend. As a destination it must fall back (without panicking),
// because only the destination is asked. As a source it is a legitimate
// candidate for the fast path: the destination decides.
type basicCopyFS struct {
	fsys       fstest.MapFS
	copyCalls  []string
	writeCalls []string
}

var (
	_ ocflfs.FS      = (*basicCopyFS)(nil)
	_ ocflfs.WriteFS = (*basicCopyFS)(nil)
	_ ocflfs.CopyFS  = (*basicCopyFS)(nil)
)

func (f *basicCopyFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	return f.fsys.Open(name)
}

func (f *basicCopyFS) Write(ctx context.Context, name string, r io.Reader) (int64, error) {
	f.writeCalls = append(f.writeCalls, name)
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	f.fsys[name] = &fstest.MapFile{Data: data}
	return int64(len(data)), nil
}

func (f *basicCopyFS) Remove(ctx context.Context, name string) error {
	delete(f.fsys, name)
	return nil
}

func (f *basicCopyFS) RemoveAll(ctx context.Context, name string) error {
	return nil
}

func (f *basicCopyFS) Copy(ctx context.Context, dst string, src string) (int64, error) {
	f.copyCalls = append(f.copyCalls, src+" -> "+dst)
	return 0, nil
}

// readOnlyFS implements only FS: no WriteFS, no CopyFS, no SameBackend.
type readOnlyFS struct {
	fsys fstest.MapFS
}

var _ ocflfs.FS = (*readOnlyFS)(nil)

func (f *readOnlyFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	return f.fsys.Open(name)
}

// TestCopy_SameBackend covers fs.Copy's decision logic: the optimized CopyFS
// path is used exactly when dstFS implements SameBackend and
// dstFS.SameBackend(srcFS) is true; otherwise Copy falls back to a manual
// read+write. The destination is the only side consulted — it is the FS that
// performs the copy, and SameBackend's contract already requires it to answer
// false when it cannot establish shared storage. In particular, the old
// `dstFS == srcFS` interface-value comparison must not drive the decision in
// either direction.
func TestCopy_SameBackend(t *testing.T) {
	const content = "some content"
	ctx := context.Background()

	t.Run("optimized path for distinct values on the same backend", func(t *testing.T) {
		// The decision must come from SameBackend, not from comparing the
		// two interface values: distinct FS values routinely describe the
		// same backend (two *BucketFS on one client and bucket), and an
		// identity comparison would silently downgrade every such copy to a
		// slow read+write. Observed via dstFS's sentinel error.
		dstFS := &spyFS{fsys: fstest.MapFS{}, same: true}
		srcFS := &spyFS{fsys: fstest.MapFS{"src-file": &fstest.MapFile{Data: []byte(content)}}, same: true}
		be.Unequal(t, srcFS, dstFS) // distinct values: an == check would miss this case
		sentinel := errors.New("observed optimized Copy path")
		dstFS.copyErr = sentinel
		_, err := ocflfs.Copy(ctx, dstFS, "dst-file", srcFS, "src-file")
		be.AllEqual(t, []string{"src-file -> dst-file"}, dstFS.copyCalls)
		be.Zero(t, dstFS.writeCalls)         // fallback must not run when the fast path is taken
		be.True(t, errors.Is(err, sentinel)) // the fast path's error must propagate
	})

	t.Run("fallback for the same value when SameBackend is false", func(t *testing.T) {
		// The mirror image: both arguments are the SAME value, which an
		// identity comparison would read as proof of a shared backend.
		// SameBackend is what decides, and it says false, so the copy must
		// fall back to a manual read+write.
		fsys := &spyFS{fsys: fstest.MapFS{"src-file": &fstest.MapFile{Data: []byte(content)}}, same: false}
		size, err := ocflfs.Copy(ctx, fsys, "dst-file", fsys, "src-file")
		be.NilErr(t, err)
		be.Equal(t, int64(len(content)), size)
		be.Zero(t, fsys.copyCalls) // Copy must not be called when SameBackend is false
		be.AllEqual(t, []string{"dst-file"}, fsys.writeCalls)
		be.True(t, bytes.Equal(fsys.fsys["dst-file"].Data, []byte(content)))
	})

	t.Run("optimized path when only dstFS implements SameBackend", func(t *testing.T) {
		// srcFS is not asked and need not implement SameBackend: requiring
		// it to would only cost a genuine same-backend pair the fast path,
		// since a destination that answers true for storage it does not
		// share is already violating the interface contract.
		dstFS := &spyFS{fsys: fstest.MapFS{}, same: true}
		srcFS := &basicCopyFS{fsys: fstest.MapFS{"src-file": &fstest.MapFile{Data: []byte(content)}}}
		sentinel := errors.New("observed optimized Copy path")
		dstFS.copyErr = sentinel
		_, err := ocflfs.Copy(ctx, dstFS, "dst-file", srcFS, "src-file")
		be.AllEqual(t, []string{"src-file -> dst-file"}, dstFS.copyCalls)
		be.Zero(t, dstFS.writeCalls)
		be.True(t, errors.Is(err, sentinel))
	})

	t.Run("fallback when dstFS does not implement SameBackend", func(t *testing.T) {
		dstFS := &basicCopyFS{fsys: fstest.MapFS{}}
		srcFS := &spyFS{fsys: fstest.MapFS{"src-file": &fstest.MapFile{Data: []byte(content)}}, same: true}
		size, err := ocflfs.Copy(ctx, dstFS, "dst-file", srcFS, "src-file")
		be.NilErr(t, err)
		be.Equal(t, int64(len(content)), size)
		be.Zero(t, dstFS.copyCalls) // the destination is the side that must answer
		be.AllEqual(t, []string{"dst-file"}, dstFS.writeCalls)
		be.True(t, bytes.Equal(dstFS.fsys["dst-file"].Data, []byte(content)))
	})

	t.Run("fallback when neither side implements SameBackend", func(t *testing.T) {
		dstFS := &basicCopyFS{fsys: fstest.MapFS{}}
		srcFS := &basicCopyFS{fsys: fstest.MapFS{"src-file": &fstest.MapFile{Data: []byte(content)}}}
		size, err := ocflfs.Copy(ctx, dstFS, "dst-file", srcFS, "src-file")
		be.NilErr(t, err)
		be.Equal(t, int64(len(content)), size)
		be.Zero(t, dstFS.copyCalls)
		be.AllEqual(t, []string{"dst-file"}, dstFS.writeCalls)
		be.True(t, bytes.Equal(dstFS.fsys["dst-file"].Data, []byte(content)))
	})

	t.Run("plain FS without WriteFS or CopyFS errors instead of panicking", func(t *testing.T) {
		// A destination that is not a WriteFS is not a panic or a silent
		// success: Copy reads the source and fails with ErrOpUnsupported
		// when it tries to write.
		dstFS := &readOnlyFS{fsys: fstest.MapFS{}}
		srcFS := &spyFS{fsys: fstest.MapFS{"src-file": &fstest.MapFile{Data: []byte(content)}}, same: true}
		_, err := ocflfs.Copy(ctx, dstFS, "dst-file", srcFS, "src-file")
		be.Nonzero(t, err)
		be.True(t, errors.Is(err, ocflfs.ErrOpUnsupported))
	})
}

// removeWalkEntry is a scripted fs.DirEntry for removeWalkFS.
type removeWalkEntry struct {
	name  string
	isDir bool
}

func (e *removeWalkEntry) Name() string { return e.name }

func (e *removeWalkEntry) IsDir() bool { return e.isDir }

func (e *removeWalkEntry) Type() fs.FileMode { return 0 }

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

// scriptedDirEntry is a fake fs.DirEntry whose Info() always fails, so a test
// can put an entry into a directory listing whose Info() returns an error.
type scriptedDirEntry struct {
	name    string
	infoErr error
}

func (e *scriptedDirEntry) Name() string { return e.name }

func (e *scriptedDirEntry) IsDir() bool { return false }

func (e *scriptedDirEntry) Type() fs.FileMode { return 0 }

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
// (e.g. fs.DirEntries itself yields (nil, err) for a non-DirEntriesFS, and a
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

// TestWalkFiles_DirEntriesErrorContract is a regression test for fileWalk's
// handling of a DirEntriesFS that yields (nil, err) pairs and entries whose
// Info() fails. Previously the (nil, err) yield was propagated but then fell
// through to path.Join(subDir, e.Name()) with a nil entry, panicking, and an
// Info() error produced a *FileRef with a nil Info field. The contract:
//   - never panic,
//   - propagate every yielded error (in order),
//   - never produce a FileRef with a nil Info field after an Info() error,
//   - keep walking valid entries after error yields.
func TestWalkFiles_DirEntriesErrorContract(t *testing.T) {
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
		// (nil, err): error with no entry — the case fs.DirEntries itself
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
