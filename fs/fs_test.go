package fs_test

import (
	"bytes"
	"context"
	"io"
	"io/fs"
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
