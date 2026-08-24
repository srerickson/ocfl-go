package fs_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// errReadDirFS is an fs.FS whose ReadDir returns configured partial entries
// together with a non-nil error, mimicking a failed directory listing that
// still produced some entries.
type errReadDirFS struct {
	entries []fs.DirEntry
	err     error
}

func (f *errReadDirFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (f *errReadDirFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return f.entries, f.err
}

var (
	_ fs.FS        = (*errReadDirFS)(nil)
	_ fs.ReadDirFS = (*errReadDirFS)(nil)
)

// TestWrapFS_DirEntries_ErrorBeforePartialEntries is a regression test: when
// the underlying fs.ReadDir returns partial entries alongside a non-nil error,
// the DirEntries iterator must yield the error as its first and only result —
// never the partial entries (an error terminates iteration). This matches the
// s3 backend behavior. Previously the iterator yielded the partial entries
// first and the error last, so a WalkFiles consumer could process files from a
// known-incomplete listing.
func TestWrapFS_DirEntries_ErrorBeforePartialEntries(t *testing.T) {
	partialEntries, err := fstest.MapFS{
		"a.txt": &fstest.MapFile{Mode: 0o644},
		"b.txt": &fstest.MapFile{Mode: 0o644},
	}.ReadDir(".")
	be.NilErr(t, err)
	readErr := errors.New("readdir: listing interrupted")
	fsys := ocflfs.NewWrapFS(&errReadDirFS{entries: partialEntries, err: readErr})

	type yield struct {
		entry fs.DirEntry
		err   error
	}
	var got []yield
	for entry, iterErr := range fsys.DirEntries(context.Background(), ".") {
		got = append(got, yield{entry: entry, err: iterErr})
	}
	// Error terminates iteration: it is the only result, and no partial
	// entries are yielded before (or after) it.
	be.Equal(t, 1, len(got))
	be.True(t, got[0].entry == nil)
	be.True(t, errors.Is(got[0].err, readErr))
}

// TestWrapFS_DirEntries_Success pins the success path: entries are yielded
// with no error at all.
func TestWrapFS_DirEntries_Success(t *testing.T) {
	fsys := ocflfs.NewWrapFS(fstest.MapFS{
		"a.txt": &fstest.MapFile{Mode: 0o644},
		"b.txt": &fstest.MapFile{Mode: 0o644},
	})
	var names []string
	var iterErr error
	for entry, err := range fsys.DirEntries(context.Background(), ".") {
		if entry != nil {
			names = append(names, entry.Name())
		}
		if err != nil {
			iterErr = err
		}
	}
	be.NilErr(t, iterErr)
	be.AllEqual(t, []string{"a.txt", "b.txt"}, names)
}
