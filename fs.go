package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

var ErrNotFile = errors.New("not a file")
var ErrFileType = errors.New("invalid file type for an OCFL context")

// FS is a minimal, read-only storage layer abstraction. It is similar to the
// standard library's io/fs.FS, except it uses contexts and OpenFile is not
// required to gracefully handle directories.
type FS interface {
	OpenFile(ctx context.Context, name string) (fs.File, error)
	ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error)
}

// WriteFS is a storage backend that supports write and remove operations.
type WriteFS interface {
	FS
	Write(ctx context.Context, name string, buffer io.Reader) (int64, error)
	// Remove the file with path name
	Remove(ctx context.Context, name string) error
	// Remove the directory with path name and all its contents. If the path
	// does not exist, return nil.
	RemoveAll(ctx context.Context, name string) error
}

// CopyFS is a storage backend that supports copying files.
type CopyFS interface {
	WriteFS
	// Copy creates or updates the file at dst with the contents of src. If dst
	// exists, it should be overwritten
	Copy(ctx context.Context, dst string, src string) error
}

// NewFS wraps an io/fs.FS as an ocfl.FS
func NewFS(fsys fs.FS) FS { return &ioFS{FS: fsys} }

// DirFS is shorthand for NewFS(os.DirFS(dir))
func DirFS(dir string) FS { return NewFS(os.DirFS(dir)) }

// ioFS wraps and fs.FS
type ioFS struct {
	fs.FS
}

// OpenFile implementes FS for ioFS
func (fsys *ioFS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, &fs.PathError{
			Op:   "openfile",
			Path: name,
			Err:  err,
		}
	}
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "openfile",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	f, err := fsys.Open(name)
	if err != nil {
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			// replace system path with name
			pathErr.Path = name
		}
		return nil, err
	}
	return f, nil
}

// ReadDir implements FS for ioFS.
func (fsys *ioFS) ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, &fs.PathError{
			Op:   "readdir",
			Path: name,
			Err:  err,
		}
	}
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "readdir",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	dirents, err := fs.ReadDir(fsys.FS, name)
	if err != nil {
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			// pathErr's Path will be system path;
			// change it to name
			pathErr.Path = name
		}
		return nil, err
	}
	return dirents, nil

}

// ReadAll returns the contents of a file.
func ReadAll(ctx context.Context, fsys FS, name string) ([]byte, error) {
	f, err := fsys.OpenFile(ctx, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// Copy copies src in srcFS to dst in dstFS. If srcFS and dstFS are the same refererence
// and it implements CopyFS, then Copy uses the fs's Copy() method.
func Copy(ctx context.Context, dstFS WriteFS, dst string, srcFS FS, src string) (err error) {
	cpFS, ok := dstFS.(CopyFS)
	if ok && dstFS == srcFS {
		if err = cpFS.Copy(ctx, dst, src); err != nil {
			err = fmt.Errorf("during copy: %w", err)
		}
		return
	}
	// otherwise, manual copy
	var srcF fs.File
	srcF, err = srcFS.OpenFile(ctx, src)
	if err != nil {
		err = fmt.Errorf("opening for copy: %w", err)
		return
	}
	defer func() {
		if closeErr := srcF.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	if _, err = dstFS.Write(ctx, dst, srcF); err != nil {
		err = fmt.Errorf("writing during copy: %w", err)
	}
	return
}

// StatFile returns file informatoin for the file name in fsys.
func StatFile(ctx context.Context, fsys FS, name string) (fs.FileInfo, error) {
	f, err := fsys.OpenFile(ctx, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}
