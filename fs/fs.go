package fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"path"
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

// DirEntriesFS is an FS that also includes the ability to read entries in a
// directory.
type DirEntriesFS interface {
	FS
	// DirEntries returns an iterator that will yield an fs.DirEntry from the named
	// directory or an error (not both). The entries should be yielded in sorted
	// order. If an error is yielded, iteration terminates.
	DirEntries(ctx context.Context, name string) iter.Seq2[fs.DirEntry, error]
}

// FileWalker is an [FS] with an optimized implementation of WalkFiles
type FileWalker interface {
	FS
	// WalkFiles returns an iterator that yields *FileRefs and/or an
	// error.
	WalkFiles(ctx context.Context, dir string) iter.Seq2[*FileRef, error]
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
	Copy(ctx context.Context, dst string, src string) (int64, error)
}

// Copy copies src in srcFS to dst in dstFS. If srcFS and dstFS are the same refererence
// and it implements CopyFS, then Copy uses the fs's Copy() method.
func Copy(ctx context.Context, dstFS FS, dst string, srcFS FS, src string) (size int64, err error) {
	cpFS, ok := dstFS.(CopyFS)
	// FIXME: better way to compare src and dst FS
	if ok && dstFS == srcFS {
		size, err = cpFS.Copy(ctx, dst, src)
		if err != nil {
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
	size, err = Write(ctx, dstFS, dst, srcF)
	if err != nil {
		err = fmt.Errorf("writing during copy: %w", err)
	}
	return
}

// DirEntries calls DirEntries if fsys implements DirEntriesFS. If fsys doesn't implement
// DirEntriesFS, it returns an iterator that yields an fs.PathError that wraps
// ErrFeatureUnsupported.
func DirEntries(ctx context.Context, fsys FS, name string) iter.Seq2[fs.DirEntry, error] {
	readDirFS, ok := fsys.(DirEntriesFS)
	if !ok {
		err := &fs.PathError{Op: "readdir", Path: name, Err: ErrOpUnsupported}
		return func(yield func(fs.DirEntry, error) bool) {
			yield(nil, err)
		}
	}
	return readDirFS.DirEntries(ctx, name)
}

// ReadDir calls DirEntries and collects all yielded directory entries in a
// slice. If an error is encountered, the slice will included all entries read
// up the point of the error.
func ReadDir(ctx context.Context, fsys FS, name string) ([]fs.DirEntry, error) {
	var entries []fs.DirEntry
	for entry, err := range DirEntries(ctx, fsys, name) {
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

// Remove checks if fsys implements WriteFS and calls its Remove method. It
// returns ErrOpUnsupported if fsys is not a WriteFS
func Remove(ctx context.Context, fsys FS, name string) error {
	writeFS, ok := fsys.(WriteFS)
	if !ok {
		return &fs.PathError{Op: "remove", Path: name, Err: ErrOpUnsupported}
	}
	return writeFS.Remove(ctx, name)
}

// RemoveAll checks if fsys implements WriteFS and calls its RemoveAll method.
// It returns ErrOpUnsupported if fsys is not a WriteFS. As a special case, if
// name == ".", RemoveAll reads the contents of the top-level directory and
// calls Remove/RemoveAll for all entries.
func RemoveAll(ctx context.Context, fsys FS, name string) error {
	writeFS, ok := fsys.(WriteFS)
	if !ok {
		return &fs.PathError{Op: "remove_all", Path: name, Err: ErrOpUnsupported}
	}
	if name != "." {
		return writeFS.RemoveAll(ctx, name)
	}
	for entry, err := range DirEntries(ctx, fsys, ".") {
		if err != nil {
			return err
		}
		var removeFn func(context.Context, FS, string) error
		switch {
		case entry.IsDir():
			removeFn = RemoveAll
		default:
			removeFn = Remove
		}
		if err := removeFn(ctx, fsys, entry.Name()); err != nil {
			return err
		}
	}
	return nil
}

// Write checks if fsys implements WriteFS and calls its Write method. It
// returns ErrOpUnsupported if fsys is not a WriteFS
func Write(ctx context.Context, fsys FS, name string, r io.Reader) (int64, error) {
	writeFS, ok := fsys.(WriteFS)
	if !ok {
		return 0, &fs.PathError{Op: "write", Path: name, Err: ErrOpUnsupported}
	}
	return writeFS.Write(ctx, name, r)
}

// StatFile returns file information for the file name in fsys.
func StatFile(ctx context.Context, fsys FS, name string) (fs.FileInfo, error) {
	f, err := fsys.OpenFile(ctx, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

// WalkFiles checks if fsys is a FileWalker and calls its WalkFiles if it is. If
// fsys isn't a FileWalker, dir is walked using [DirEntries].
func WalkFiles(ctx context.Context, fsys FS, dir string) iter.Seq2[*FileRef, error] {
	if walkFS, ok := fsys.(FileWalker); ok {
		return walkFS.WalkFiles(ctx, dir)
	}
	return func(yield func(*FileRef, error) bool) {
		fileWalk(ctx, fsys, dir, ".", yield)
	}
}

func fileWalk(ctx context.Context, fsys FS, walkRoot string, subDir string, yield func(*FileRef, error) bool) bool {
	for e, err := range DirEntries(ctx, fsys, path.Join(walkRoot, subDir)) {
		if err != nil {
			if !yield(nil, err) {
				return false
			}
		}
		entryPath := path.Join(subDir, e.Name())
		switch {
		case e.IsDir():
			if !fileWalk(ctx, fsys, walkRoot, entryPath, yield) {
				return false
			}
		default:
			info, err := e.Info()
			if err != nil {
				if !yield(nil, err) {
					return false
				}
			}
			ref := &FileRef{
				FS:      fsys,
				BaseDir: walkRoot,
				Path:    entryPath,
				Info:    info,
			}
			if !yield(ref, nil) {
				return false
			}
		}
	}
	return true
}
