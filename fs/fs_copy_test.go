package fs_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
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
// SameBackend. fs.Copy must fall back for it (without panicking) because the
// fast path requires both sides to confirm they share a backend.
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
// path is used exactly when both sides implement SameBackend and
// dstFS.SameBackend(srcFS) is true; otherwise Copy falls back to a manual
// read+write. In particular, the old `dstFS == srcFS` interface-value
// comparison must not drive the decision in either direction.
func TestCopy_SameBackend(t *testing.T) {
	const content = "some content"
	ctx := context.Background()

	t.Run("optimized path for distinct values on the same backend", func(t *testing.T) {
		// Regression for the bug behind SameBackend: the old
		// dstFS == srcFS comparison compared two distinct *BucketFS
		// values unequal and silently fell back to a slow read+write.
		// Two distinct FS values that confirm the same backend must take
		// dstFS.Copy() (observed via dstFS's sentinel error).
		dstFS := &spyFS{fsys: fstest.MapFS{}, same: true}
		srcFS := &spyFS{fsys: fstest.MapFS{"src-file": &fstest.MapFile{Data: []byte(content)}}, same: true}
		be.Unequal(t, srcFS, dstFS) // the old == comparison would have missed this case
		sentinel := errors.New("observed optimized Copy path")
		dstFS.copyErr = sentinel
		_, err := ocflfs.Copy(ctx, dstFS, "dst-file", srcFS, "src-file")
		be.AllEqual(t, []string{"src-file -> dst-file"}, dstFS.copyCalls)
		be.Zero(t, dstFS.writeCalls)         // fallback must not run when the fast path is taken
		be.True(t, errors.Is(err, sentinel)) // the fast path's error must propagate
	})

	t.Run("fallback for the same value when SameBackend is false", func(t *testing.T) {
		// Regression: the old dstFS == srcFS comparison would have taken
		// the optimized path here because both arguments are the SAME
		// value. With SameBackend false the copy must fall back to a
		// manual read+write.
		fsys := &spyFS{fsys: fstest.MapFS{"src-file": &fstest.MapFile{Data: []byte(content)}}, same: false}
		size, err := ocflfs.Copy(ctx, fsys, "dst-file", fsys, "src-file")
		be.NilErr(t, err)
		be.Equal(t, int64(len(content)), size)
		be.Zero(t, fsys.copyCalls) // Copy must not be called when SameBackend is false
		be.AllEqual(t, []string{"dst-file"}, fsys.writeCalls)
		be.True(t, bytes.Equal(fsys.fsys["dst-file"].Data, []byte(content)))
	})

	t.Run("fallback when srcFS does not implement SameBackend", func(t *testing.T) {
		dstFS := &spyFS{fsys: fstest.MapFS{}, same: true}
		srcFS := &basicCopyFS{fsys: fstest.MapFS{"src-file": &fstest.MapFile{Data: []byte(content)}}}
		size, err := ocflfs.Copy(ctx, dstFS, "dst-file", srcFS, "src-file")
		be.NilErr(t, err)
		be.Equal(t, int64(len(content)), size)
		be.Zero(t, dstFS.copyCalls) // fast path requires BOTH sides to implement SameBackend
		be.AllEqual(t, []string{"dst-file"}, dstFS.writeCalls)
		be.True(t, bytes.Equal(dstFS.fsys["dst-file"].Data, []byte(content)))
	})

	t.Run("fallback when dstFS does not implement SameBackend", func(t *testing.T) {
		dstFS := &basicCopyFS{fsys: fstest.MapFS{}}
		srcFS := &spyFS{fsys: fstest.MapFS{"src-file": &fstest.MapFile{Data: []byte(content)}}, same: true}
		size, err := ocflfs.Copy(ctx, dstFS, "dst-file", srcFS, "src-file")
		be.NilErr(t, err)
		be.Equal(t, int64(len(content)), size)
		be.Zero(t, dstFS.copyCalls) // fast path requires BOTH sides to implement SameBackend
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
