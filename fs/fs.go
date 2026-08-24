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
	// Write writes the contents of buffer to the file with path name,
	// replacing any existing file, and returns the number of bytes
	// written. Implementations should make the write atomic where
	// possible: name must never be observable with partially written
	// contents, even if Write fails, its context is canceled, or the
	// process crashes mid-write. The local filesystem implementation
	// writes to a temporary file in the target's directory and renames
	// it over name only after the data is fully written and synced, so
	// a partial file is never visible (see fs/local). The s3
	// implementation uploads the full object before it becomes visible
	// at name. An implementation that cannot provide atomicity should
	// document that limitation in its Write documentation.
	Write(ctx context.Context, name string, buffer io.Reader) (int64, error)
	// Remove removes the file with path name.
	//
	// Contract: removing a file that does not exist returns an error that
	// satisfies errors.Is(err, fs.ErrNotExist), so callers can reliably
	// detect a missing file regardless of backend. Implementations must
	// account for idempotent deletes in the underlying store: the S3
	// backend checks existence with a HEAD request before DeleteObject
	// (which alone would silently succeed for missing keys), and the local
	// backend surfaces the "not exist" error from os.Remove.
	//
	// Removing the top-level directory (".") is always an error and must
	// not affect the storage root. The exact error is backend-specific: the
	// S3 backend reports fs.ErrNotExist, while the local backend returns a
	// *fs.PathError with a descriptive message. Name "." is the only name
	// for which this contract permits a backend-dependent error.
	Remove(ctx context.Context, name string) error
	// RemoveAll removes the directory with path name and all its contents.
	// Unlike Remove, it is idempotent: if the path does not exist, it
	// returns nil.
	RemoveAll(ctx context.Context, name string) error
}

// CopyFS is a storage backend that supports copying files.
type CopyFS interface {
	WriteFS
	// Copy creates or updates the file at dst with the contents of src. If dst
	// exists, it should be overwritten
	Copy(ctx context.Context, dst string, src string) (int64, error)
}

// SameBackend is an optional interface that an [FS] implementation can use to
// report whether two FS values refer to the same underlying storage backend.
// [Copy] uses it to decide whether the optimized [CopyFS] copy path is safe to
// use when copying from srcFS to dstFS.
type SameBackend interface {
	// SameBackend returns true if other refers to the same underlying storage
	// backend as the receiver. Implementations must return false if they
	// cannot determine that other shares the receiver's backend; never assume.
	SameBackend(other FS) bool
}

// Copy copies src in srcFS to dst in dstFS. If dstFS implements CopyFS and
// both srcFS and dstFS implement [SameBackend] and dstFS.SameBackend(srcFS)
// returns true (i.e. the two FS values refer to the same underlying storage),
// Copy uses dstFS's Copy() method. Otherwise, Copy falls back to opening src
// in srcFS and writing it to dst in dstFS.
func Copy(ctx context.Context, dstFS FS, dst string, srcFS FS, src string) (size int64, err error) {
	cpFS, ok := dstFS.(CopyFS)
	if ok {
		// Use the destination FS's Copy() only when both srcFS and dstFS
		// implement SameBackend and dstFS confirms that srcFS refers to the
		// same underlying storage. dstFS is the receiver because it is the
		// FS that will perform the copy.
		// FIXME: FS types that don't implement SameBackend always take the
		// slow path, even when srcFS and dstFS refer to the same backend.
		dstSB, dstOK := dstFS.(SameBackend)
		_, srcOK := srcFS.(SameBackend)
		if dstOK && srcOK && dstSB.SameBackend(srcFS) {
			size, err = cpFS.Copy(ctx, dst, src)
			if err != nil {
				err = fmt.Errorf("during copy: %w", err)
			}
			return
		}
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
// returns ErrOpUnsupported if fsys is not a WriteFS. See WriteFS.Remove for
// the contract on missing files and the top-level directory.
func Remove(ctx context.Context, fsys FS, name string) error {
	writeFS, ok := fsys.(WriteFS)
	if !ok {
		return &fs.PathError{Op: "remove", Path: name, Err: ErrOpUnsupported}
	}
	return writeFS.Remove(ctx, name)
}

// RemoveAll checks if fsys implements WriteFS and calls its RemoveAll method.
// It returns ErrOpUnsupported if fsys is not a WriteFS. As a special case, if
// name == ".", RemoveAll removes the contents of the top-level directory
// without removing the directory itself, preferring the backend's own
// RemoveAll when it can empty the root (the S3 backend lists the entire
// bucket and batch-deletes every object). A backend that must not remove its
// own root (the local backend, whose storage root must survive) refuses with
// an error, and RemoveAll falls back to removing the top-level entries one by
// one. That fallback is best-effort: a failed entry does not stop the walk,
// all per-entry errors are joined, and a partial deletion may remain when an
// error is returned. Directory recursion threads the accumulated prefix (see
// path.Join) instead of passing bare entry names, so the walk does not assume
// DirEntries(".") yields top-level basenames.
func RemoveAll(ctx context.Context, fsys FS, name string) error {
	writeFS, ok := fsys.(WriteFS)
	if !ok {
		return &fs.PathError{Op: "remove_all", Path: name, Err: ErrOpUnsupported}
	}
	if name != "." {
		return writeFS.RemoveAll(ctx, name)
	}
	// Prefer the backend's RemoveAll for ".": when supported (e.g. S3, whose
	// removeAll with name "." lists the whole bucket and deletes every object
	// with batched DeleteObjects requests), it is both faster and atomic per
	// batch than per-entry deletes. Backends that refuse to remove the
	// top-level directory (the local backend, which must never delete the
	// storage root) return an error here; fall through to the per-entry walk.
	// The refusal error itself is not joined below: it describes the backend's
	// guard, not a failed entry, and the walk either completes the removal
	// (nil) or reports the entries that actually failed.
	if err := writeFS.RemoveAll(ctx, "."); err == nil {
		return nil
	}
	return removeRootEntries(ctx, fsys, ".")
}

// removeRootEntries removes the contents of the directory at dir (the
// top-level directory when dir == ".") entry by entry, recursing into
// subdirectories. It never removes dir itself: for the "." case the
// top-level directory is the storage root and must survive. Removal is
// best-effort: per-entry errors are collected with errors.Join and remaining
// entries are still attempted, so a partial deletion is always reported.
// Names passed to Remove/RemoveAll are full relative paths (path.Join of the
// accumulated prefix and the entry name); DirEntries yields names relative
// to the directory being listed, which may be basenames or full relative
// paths, and the join is correct for both.
func removeRootEntries(ctx context.Context, fsys FS, dir string) error {
	var errs []error
	for entry, err := range DirEntries(ctx, fsys, dir) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if entry == nil {
			// Defensive: a nil entry without an error violates the
			// DirEntriesFS contract, but must not cause a panic.
			continue
		}
		entryPath := path.Join(dir, entry.Name())
		var removeFn func(context.Context, FS, string) error
		switch {
		case entry.IsDir():
			removeFn = RemoveAll
		default:
			removeFn = Remove
		}
		if err := removeFn(ctx, fsys, entryPath); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
			// The DirEntriesFS contract permits yielding (nil, err) pairs
			// (e.g. fs.DirEntries does this when fsys isn't a DirEntriesFS).
			// Propagate the error and move on; never touch e when it may be nil.
			if !yield(nil, err) {
				return false
			}
			continue
		}
		if e == nil {
			// Defensive: an iterator that yields a nil entry without an
			// error violates the DirEntriesFS contract, but must not
			// cause a panic.
			continue
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
				continue
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
