package fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"slices"
	"strings"
)

var (
	ErrOpUnsupported = errors.New("operation not supported by the file system")
	ErrNotFile       = errors.New("not a file")
	ErrFileType      = errors.New("invalid file type for an OCFL context")
)

// FS is the minimal file system abstraction that includes the ability to read
// named files (not directories).
type FS interface {
	// OpenFile opens the named file for reading. It is like [io/fs.FS.Open],
	// except it returns an error if name is a directory.
	OpenFile(ctx context.Context, name string) (fs.File, error)
}

// ReadDirFS is an FS that also includes the ability to read
// entries in a directory.
type ReadDirFS interface {
	FS
	// ReadDir returns an iterator that will yield an fs.DirEntry from the named
	// directory or an error (not both). The entries should be yielded in sorted
	// order. If an error is yielded, iteration terminates.
	ReadDir(ctx context.Context, name string) iter.Seq2[fs.DirEntry, error]
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

// ReadDir calls ReadDir if fsys implements ReadDirFS. If fsys doesn't implement
// ReadDirFS, it returns an iterator that yields an fs.PathError that wraps
// ErrFeatureUnsupported.
func ReadDir(ctx context.Context, fsys FS, name string) iter.Seq2[fs.DirEntry, error] {
	readDirFS, ok := fsys.(ReadDirFS)
	if !ok {
		err := &fs.PathError{Op: "readdir", Path: name, Err: ErrOpUnsupported}
		return func(yield func(fs.DirEntry, error) bool) {
			yield(nil, err)
		}
	}
	return readDirFS.ReadDir(ctx, name)
}

// ReadDirCollect calls ReadDir and collects all yielded directory entries in a
// slice. If an error is encountered, the slice will included all entries read
// up the point of the error.
func ReadDirCollect(ctx context.Context, fsys FS, name string) ([]fs.DirEntry, error) {
	var entries []fs.DirEntry
	for entry, err := range ReadDir(ctx, fsys, name) {
		if entry != nil {
			entries = append(entries, entry)
		}
		if err != nil {
			return entries, err
		}
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return entries, nil
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

// FileWalker is an [FS] with an optimized implementation of WalkFiles
type FileWalker interface {
	FS
	// WalkFiles returns an iterator that yields *FileRefs and/or an
	// error.
	WalkFiles(ctx context.Context, dir string) iter.Seq2[*FileRef, error]
}
