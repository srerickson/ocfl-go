package fs_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"iter"
	"testing"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// spySrcFS serves body from OpenFile, regardless of the name requested. It's
// used as srcFS in Copy dispatch tests, where only the dispatch decision --
// not the name resolution -- is under test.
type spySrcFS struct {
	body []byte
}

func (s *spySrcFS) OpenFile(_ context.Context, _ string) (fs.File, error) {
	return &spyFile{r: bytes.NewReader(s.body)}, nil
}

type spyFile struct {
	r *bytes.Reader
}

func (f *spyFile) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *spyFile) Close() error               { return nil }
func (f *spyFile) Stat() (fs.FileInfo, error) { return nil, fs.ErrInvalid }

var _ ocflfs.FS = (*spySrcFS)(nil)

// spyCopyFS is a WriteFS/CopyFS spy used as dstFS in Copy dispatch tests. It
// does not implement SameBackend.
type spyCopyFS struct {
	written    bytes.Buffer
	copyCalled bool
}

func (d *spyCopyFS) OpenFile(context.Context, string) (fs.File, error) { return nil, fs.ErrNotExist }

func (d *spyCopyFS) Write(_ context.Context, _ string, r io.Reader) (int64, error) {
	return io.Copy(&d.written, r)
}

func (d *spyCopyFS) Remove(context.Context, string) error    { return nil }
func (d *spyCopyFS) RemoveAll(context.Context, string) error { return nil }

func (d *spyCopyFS) Copy(context.Context, string, string) (int64, error) {
	d.copyCalled = true
	return 0, nil
}

var (
	_ ocflfs.FS     = (*spyCopyFS)(nil)
	_ ocflfs.CopyFS = (*spyCopyFS)(nil)
)

// spySameBackendFS wraps spyCopyFS and additionally implements SameBackend,
// reporting whatever result is configured.
type spySameBackendFS struct {
	spyCopyFS
	result bool
}

func (d *spySameBackendFS) SameBackend(ocflfs.FS) bool { return d.result }

var _ ocflfs.SameBackend = (*spySameBackendFS)(nil)

func TestCopyDispatch(t *testing.T) {
	ctx := context.Background()
	src := &spySrcFS{body: []byte("hello")}

	t.Run("same backend uses dstFS.Copy", func(t *testing.T) {
		dst := &spySameBackendFS{result: true}
		_, err := ocflfs.Copy(ctx, dst, "dst", src, "src")
		be.NilErr(t, err)
		be.True(t, dst.copyCalled)
		be.Equal(t, 0, dst.written.Len())
	})

	t.Run("different backend falls back to write", func(t *testing.T) {
		dst := &spySameBackendFS{result: false}
		_, err := ocflfs.Copy(ctx, dst, "dst", src, "src")
		be.NilErr(t, err)
		be.False(t, dst.copyCalled)
		be.Equal(t, "hello", dst.written.String())
	})

	t.Run("dstFS without SameBackend falls back to write", func(t *testing.T) {
		dst := &spyCopyFS{}
		_, err := ocflfs.Copy(ctx, dst, "dst", src, "src")
		be.NilErr(t, err)
		be.False(t, dst.copyCalled)
		be.Equal(t, "hello", dst.written.String())
	})
}

// noncmpFS is a WriteFS/CopyFS whose dynamic type carries a map field, so it
// is not comparable with ==. It reproduces the original defect: the previous
// dstFS == srcFS comparison panicked comparing two values of the same
// non-comparable dynamic type, which happens whenever the same *noncmpFS is
// used as both src and dst.
type noncmpFS struct {
	m map[string]string
}

func (n *noncmpFS) OpenFile(context.Context, string) (fs.File, error) {
	return &spyFile{r: bytes.NewReader([]byte("x"))}, nil
}

func (n *noncmpFS) Write(_ context.Context, _ string, r io.Reader) (int64, error) {
	return io.Copy(io.Discard, r)
}

func (n *noncmpFS) Remove(context.Context, string) error    { return nil }
func (n *noncmpFS) RemoveAll(context.Context, string) error { return nil }

func (n *noncmpFS) Copy(context.Context, string, string) (int64, error) {
	return 0, nil
}

var (
	_ ocflfs.FS     = (*noncmpFS)(nil)
	_ ocflfs.CopyFS = (*noncmpFS)(nil)
)

func TestCopyDispatch_NonComparableDynamicType(t *testing.T) {
	ctx := context.Background()
	fsys := &noncmpFS{m: map[string]string{}}
	// fsys does not implement SameBackend, so Copy falls back to the manual
	// path regardless of dst/src identity -- the point of the test is that
	// it does not panic getting there.
	_, err := ocflfs.Copy(ctx, fsys, "dst", fsys, "src")
	be.NilErr(t, err)
}

// stubDirEntry is a minimal fs.DirEntry for exercising the RemoveAll(".")
// loop without a real backend.
type stubDirEntry struct {
	name  string
	isDir bool
}

func (e *stubDirEntry) Name() string { return e.name }
func (e *stubDirEntry) IsDir() bool  { return e.isDir }
func (e *stubDirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}
func (e *stubDirEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrInvalid }

// rootStubFS is a WriteFS/DirEntriesFS spy whose root entries and per-name
// removal errors are configured directly, used to exercise the
// package-level RemoveAll(".") loop.
type rootStubFS struct {
	entries    []fs.DirEntry
	entriesErr error // if set, yielded after entries, with a nil entry
	yieldNil   bool  // if true, also yield a (nil, nil) pair before entriesErr/entries end

	removeErr map[string]error // per-name error returned by Remove/RemoveAll

	removed []string // names actually passed to Remove/RemoveAll, in order
}

func (r *rootStubFS) OpenFile(context.Context, string) (fs.File, error) { return nil, fs.ErrNotExist }

func (r *rootStubFS) DirEntries(_ context.Context, _ string) iter.Seq2[fs.DirEntry, error] {
	return func(yield func(fs.DirEntry, error) bool) {
		if r.yieldNil {
			if !yield(nil, nil) {
				return
			}
		}
		for _, e := range r.entries {
			if !yield(e, nil) {
				return
			}
		}
		if r.entriesErr != nil {
			yield(nil, r.entriesErr)
		}
	}
}

func (r *rootStubFS) Write(context.Context, string, io.Reader) (int64, error) {
	return 0, fs.ErrInvalid
}

func (r *rootStubFS) Remove(_ context.Context, name string) error {
	r.removed = append(r.removed, name)
	return r.removeErr[name]
}

func (r *rootStubFS) RemoveAll(_ context.Context, name string) error {
	r.removed = append(r.removed, name)
	return r.removeErr[name]
}

var (
	_ ocflfs.FS           = (*rootStubFS)(nil)
	_ ocflfs.WriteFS      = (*rootStubFS)(nil)
	_ ocflfs.DirEntriesFS = (*rootStubFS)(nil)
)

func TestRemoveAllDot(t *testing.T) {
	ctx := context.Background()

	t.Run("attempts every sibling and joins the errors", func(t *testing.T) {
		boom := errors.New("boom")
		fsys := &rootStubFS{
			entries: []fs.DirEntry{
				&stubDirEntry{name: "adir", isDir: true},
				&stubDirEntry{name: "boom.txt"},
				&stubDirEntry{name: "keep.txt"},
			},
			removeErr: map[string]error{"boom.txt": boom},
		}
		err := ocflfs.RemoveAll(ctx, fsys, ".")
		be.True(t, err != nil)
		be.True(t, errors.Is(err, boom))
		// Every sibling must have been attempted, not just the ones before
		// the failure.
		be.AllEqual(t, []string{"adir", "boom.txt", "keep.txt"}, fsys.removed)
	})

	t.Run("skips a nil entry paired with a nil error without panicking", func(t *testing.T) {
		fsys := &rootStubFS{
			yieldNil: true,
			entries:  []fs.DirEntry{&stubDirEntry{name: "keep.txt"}},
		}
		err := ocflfs.RemoveAll(ctx, fsys, ".")
		be.NilErr(t, err)
		be.AllEqual(t, []string{"keep.txt"}, fsys.removed)
	})

	t.Run("joins a DirEntries iteration error with per-entry errors", func(t *testing.T) {
		boom := errors.New("boom")
		iterErr := errors.New("iteration failed")
		fsys := &rootStubFS{
			entries:    []fs.DirEntry{&stubDirEntry{name: "boom.txt"}},
			removeErr:  map[string]error{"boom.txt": boom},
			entriesErr: iterErr,
		}
		err := ocflfs.RemoveAll(ctx, fsys, ".")
		be.True(t, errors.Is(err, boom))
		be.True(t, errors.Is(err, iterErr))
	})
}

// rootRemoverStubFS is a WriteFS additionally implementing RootRemover, used
// to assert that the package-level RemoveAll(".") prefers RemoveRoot and
// never falls back to a per-entry loop.
type rootRemoverStubFS struct {
	rootStubFS
	removeRootCalls int
	removeRootErr   error
}

func (r *rootRemoverStubFS) RemoveRoot(context.Context) error {
	r.removeRootCalls++
	return r.removeRootErr
}

var _ ocflfs.RootRemover = (*rootRemoverStubFS)(nil)

func TestRemoveAllDot_PrefersRootRemover(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")
	fsys := &rootRemoverStubFS{
		rootStubFS: rootStubFS{
			entries: []fs.DirEntry{&stubDirEntry{name: "boom.txt"}},
		},
		removeRootErr: boom,
	}
	err := ocflfs.RemoveAll(ctx, fsys, ".")
	// The error is returned exactly as RemoveRoot reported it.
	be.True(t, errors.Is(err, boom))
	be.Equal(t, 1, fsys.removeRootCalls)
	// No per-entry fallback: DirEntries/Remove is never consulted.
	be.Equal(t, 0, len(fsys.removed))
}
