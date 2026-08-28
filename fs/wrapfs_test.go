package fs_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// fakeDirEntry is a minimal fs.DirEntry, just enough to be told apart by
// name.
type fakeDirEntry struct{ name string }

func (e fakeDirEntry) Name() string               { return e.name }
func (e fakeDirEntry) IsDir() bool                { return false }
func (e fakeDirEntry) Type() fs.FileMode          { return 0 }
func (e fakeDirEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrInvalid }

// partialReadDirFile is an fs.ReadDirFile whose ReadDir(-1) call returns a
// fixed slice of entries and a fixed error together, reproducing the
// partial-listing shape [io/fs.ReadDirFile] documents for a real directory
// handle that fails partway through a read. It does not implement
// fs.ReadDirFS at the containing FS level, so fs.ReadDir reaches it through
// Open + ReadDir(-1) -- the same path WrapFS.DirEntries takes for any wrapped
// fs.FS that isn't itself a ReadDirFS (os.DirFS is; this fake deliberately
// isn't, to exercise the fallback).
type partialReadDirFile struct {
	entries []fs.DirEntry
	err     error
}

func (f *partialReadDirFile) Stat() (fs.FileInfo, error) { return nil, fs.ErrInvalid }
func (f *partialReadDirFile) Read([]byte) (int, error)   { return 0, fs.ErrInvalid }
func (f *partialReadDirFile) Close() error               { return nil }
func (f *partialReadDirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	return f.entries, f.err
}

var _ fs.ReadDirFile = (*partialReadDirFile)(nil)

// partialReadDirFS opens dirFile for any name, standing in for a wrapped
// fs.FS whose one directory of interest fails partway through a listing.
type partialReadDirFS struct {
	dirFile *partialReadDirFile
}

func (f *partialReadDirFS) Open(string) (fs.File, error) { return f.dirFile, nil }

var _ fs.FS = (*partialReadDirFS)(nil)

func namesOf(entries []fs.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// collectDirEntries drains a DirEntries iterator, returning every entry
// yielded and the first error, mirroring how ocflfs.ReadDir consumes it.
func collectDirEntries(seq func(func(fs.DirEntry, error) bool)) ([]fs.DirEntry, error) {
	var entries []fs.DirEntry
	var err error
	for entry, e := range seq {
		if entry != nil {
			entries = append(entries, entry)
		}
		if e != nil {
			err = e
			break
		}
	}
	return entries, err
}

func TestWrapFSDirEntriesPartialListing(t *testing.T) {
	// The defect reported in issue 176: a wrapped fs.FS whose ReadDir(-1)
	// returns entries and an error together used to have those entries
	// yielded first and the error last, in the same sequence -- which is
	// exactly what DirEntriesFS now documents as the expected shape (see
	// fs/fs.go), not a bug. This pins the sequence exactly: the entries, in
	// order, then the one error, then nothing further.
	readErr := errors.New("read failed partway")
	fsys := ocflfs.NewWrapFS(&partialReadDirFS{
		dirFile: &partialReadDirFile{
			entries: []fs.DirEntry{fakeDirEntry{"a"}, fakeDirEntry{"b"}},
			err:     readErr,
		},
	})

	entries, err := collectDirEntries(fsys.DirEntries(context.Background(), "dir"))
	be.AllEqual(t, []string{"a", "b"}, namesOf(entries))
	// WrapFS yields whatever fs.ReadDir returns as the terminal error
	// verbatim -- it wraps in *fs.PathError only where it constructs the
	// error itself (invalid path, cancellation below), not this tail case.
	be.Equal(t, readErr, err)
}

func TestWrapFSDirEntriesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("during a listing with no read error", func(t *testing.T) {
		fsys := ocflfs.NewWrapFS(&partialReadDirFS{
			dirFile: &partialReadDirFile{
				entries: []fs.DirEntry{fakeDirEntry{"a"}},
			},
		})
		entries, err := collectDirEntries(fsys.DirEntries(ctx, "dir"))
		be.Equal(t, 0, len(entries))
		be.True(t, errors.Is(err, context.Canceled))

		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "readdir", pathErr.Op)
		be.Equal(t, "dir", pathErr.Path)
		// Nothing else is joined in: the cause is exactly context.Canceled.
		be.Equal(t, context.Canceled, pathErr.Err)
	})

	t.Run("during a failed listing, both errors survive", func(t *testing.T) {
		// A canceled context must not shadow a pending ReadDir error: a
		// caller matching either context.Canceled or the read failure with
		// errors.Is should find it. This is the secondary defect named in
		// issue 176.
		readErr := errors.New("read failed partway")
		fsys := ocflfs.NewWrapFS(&partialReadDirFS{
			dirFile: &partialReadDirFile{
				entries: []fs.DirEntry{fakeDirEntry{"a"}},
				err:     readErr,
			},
		})
		entries, err := collectDirEntries(fsys.DirEntries(ctx, "dir"))
		be.Equal(t, 0, len(entries))
		be.True(t, errors.Is(err, context.Canceled))
		be.True(t, errors.Is(err, readErr))
	})
}

func TestWrapFSDirEntriesInvalidPath(t *testing.T) {
	fsys := ocflfs.NewWrapFS(&partialReadDirFS{dirFile: &partialReadDirFile{}})
	for _, dir := range []string{"../escape", "/absolute", "a/../b", ""} {
		_, err := collectDirEntries(fsys.DirEntries(context.Background(), dir))
		be.True(t, err != nil)
		be.True(t, errors.Is(err, fs.ErrInvalid))
	}
}

func TestWrapFSDirEntriesHappyPath(t *testing.T) {
	// The real os.DirFS route: os.DirFS implements fs.ReadDirFS, so
	// fs.ReadDir(fsys.FS, name) takes the single-call path instead of the
	// Open+ReadDir(-1) fallback the fakes above exercise. Sorted order and a
	// clean, error-free listing are asserted here.
	dir := t.TempDir()
	for _, name := range []string{"zeta.txt", "alpha.txt"} {
		be.NilErr(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644))
	}
	fsys := ocflfs.DirFS(dir)

	entries, err := collectDirEntries(fsys.DirEntries(context.Background(), "."))
	be.NilErr(t, err)
	be.AllEqual(t, []string{"alpha.txt", "zeta.txt"}, namesOf(entries))
}
