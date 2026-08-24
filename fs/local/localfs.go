package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"unicode/utf8"

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

const (
	dirPerm = 0755
	// tempPerm is the creation mode for the temp file used by Write's
	// atomic-write sequence. It is subject to the process umask, so a new
	// file lands at 0666 &^ umask (typically 0644), matching plain
	// os.Create semantics. If the target already exists, its permissions
	// are copied onto the temp file with chmod before the rename instead.
	tempPerm = 0666
)

type FS struct {
	ocflfs.DirEntriesFS
	// path is os-specific path to a directory
	path string
}

var _ ocflfs.WriteFS = (*FS)(nil)
var _ ocflfs.DirEntriesFS = (*FS)(nil)
var _ ocflfs.SameBackend = (*FS)(nil)

func NewFS(path string) (*FS, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("new backend: %w", err)
	}
	return &FS{
		path:         abs,
		DirEntriesFS: ocflfs.NewWrapFS(os.DirFS(abs)),
	}, nil
}

func (fsys *FS) Root() string {
	return fsys.path
}

// SameBackend implements [ocflfs.SameBackend]: it returns true only if other is
// also a *FS and its root path resolves to the same directory as fsys's root.
// Both root paths are absolutized with filepath.Abs (which also cleans them)
// before comparing, so trailing slashes, ".", "..", and relative paths don't
// cause false negatives. If the paths cannot be resolved, it returns false.
func (fsys *FS) SameBackend(other ocflfs.FS) bool {
	otherLocal, ok := other.(*FS)
	if !ok {
		return false
	}
	thisRoot, err := filepath.Abs(fsys.path)
	if err != nil {
		return false
	}
	otherRoot, err := filepath.Abs(otherLocal.path)
	if err != nil {
		return false
	}
	return thisRoot == otherRoot
}

// Write writes the contents of src to the file named name, creating the
// file and any missing parent directories, and returns the number of
// bytes written. The write is atomic: src is copied to a unique temporary
// file (.<base>.tmp-<random>) in the target's own directory, synced to
// disk, and renamed over name only when the copy is complete. Readers of
// name therefore never observe a partial file — a crash, a canceled
// context, or any other failure leaves either the old file or no file at
// name, never a truncated one. On any failure before the rename the
// temporary file is removed (best-effort), so failed writes do not leak
// temp files. If name already exists, its permissions are preserved on
// the replacement; otherwise the new file is created with mode 0666
// subject to the process umask.
func (fsys *FS) Write(ctx context.Context, name string, src io.Reader) (int64, error) {
	fullPath, err := fsys.osPath(name)
	if err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	parent := filepath.Dir(fullPath)
	if err := os.MkdirAll(parent, dirPerm); err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	// Preserve the target's permissions when it already exists: the temp
	// file is chmod'd to the exact mode (chmod is not umask-masked). For a
	// new file, the temp file is left at tempPerm &^ umask, matching plain
	// os.Create semantics. The target is examined with Lstat, which does
	// not follow symlinks: a symlinked target contributes its own mode
	// (typically 0777 on POSIX), never the referent's — the rename below
	// replaces the link entry itself, so preserving the referent's mode
	// would silently stamp the referent's permissions onto a brand-new
	// regular file. Lstat errors are tolerated: a missing target simply
	// means a new file created with the default temp mode.
	var preserveMode fs.FileMode
	if info, err := os.Lstat(fullPath); err == nil {
		preserveMode = info.Mode().Perm()
	}
	// Write to a unique temp file in the same directory as the target: the
	// final rename is then atomic (same filesystem on POSIX and Windows)
	// and no partial content ever appears at fullPath. tempFileName names
	// it .<base>.tmp-<random>, and O_CREATE|O_EXCL at creation guarantees
	// the name is fresh even if another writer races in the same directory.
	tmpPath, tmp, err := createTempFile(parent, fullPath)
	if err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	// Until the rename succeeds, every error path removes the temp file
	// (best-effort) so failed or canceled writes never leak temp files.
	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()        // no-op if already closed
			_ = os.Remove(tmpPath) // no-op if already renamed away
		}
	}()
	if preserveMode != 0 {
		if err := tmp.Chmod(preserveMode); err != nil {
			return 0, &fs.PathError{
				Op:   "write",
				Path: name,
				Err:  err,
			}
		}
	}
	n, err := copyWithContext(ctx, tmp, src)
	if err != nil {
		// If the copy failed because the context was canceled or expired,
		// report that instead of the underlying read/write error.
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return n, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	// fsync the temp file before it becomes visible at the final path so a
	// crash can't leave a zero-length or truncated target.
	if err := tmp.Sync(); err != nil {
		return n, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	if err := tmp.Close(); err != nil {
		return n, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	// Atomic swap: readers of fullPath see either the old file or the new
	// complete file, never a partial one.
	if err := os.Rename(tmpPath, fullPath); err != nil {
		return n, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	committed = true
	// Best-effort fsync of the containing directory persists the rename
	// itself across a crash. Failures are ignored: the write has already
	// succeeded, and not every filesystem supports fsync on a directory.
	if dir, err := os.Open(parent); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return n, nil
}

// tempFileName returns the name of a temporary file in the same directory as
// target: .<base>.tmp-<random>. The leading dot keeps temp files out of
// normal directory listings, and the random suffix (enforced with O_EXCL at
// creation) prevents collisions between concurrent writers. The base is
// truncated when needed so the full name stays within NAME_MAX (255 bytes),
// which matters for long OCFL content names. Truncation backs off to a UTF-8
// rune boundary: slicing raw bytes at the limit can split a multibyte rune
// in half, producing an invalid name that strict filesystems reject and that
// breaks the name-prefix correlation between the temp file and its target.
func tempFileName(target string) string {
	base := filepath.Base(target)
	const reserved = len(".") + len(".tmp-") + 16 // leading dot, suffix, hex random
	if max := 255 - reserved; len(base) > max {
		// Walk back from the byte limit to the nearest byte that starts a
		// rune, so the slice never ends in the middle of a UTF-8 sequence.
		// base[max] is always in range here (len(base) > max), and the
		// cut > 0 guard stops the scan at the front of the string, so
		// base[:cut] is a valid UTF-8 prefix for any valid UTF-8 input.
		cut := max
		for cut > 0 && !utf8.RuneStart(base[cut]) {
			cut--
		}
		base = base[:cut]
	}
	return "." + base + ".tmp-" + fmt.Sprintf("%016x", rand.Uint64())
}

// createTempFile creates a new file in dir named after target with
// O_CREATE|O_EXCL|O_WRONLY, so the file is guaranteed not to clobber an
// existing file. If the exclusive create collides with an existing file it
// retries with a fresh random name (practically impossible). The file mode
// is tempPerm, subject to the process umask.
func createTempFile(dir, target string) (string, *os.File, error) {
	for range 10 {
		path := filepath.Join(dir, tempFileName(target))
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, tempPerm)
		if err == nil {
			return path, f, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", nil, err
		}
	}
	return "", nil, errors.New("unable to create a unique temp file")
}

// copyWithContext copies src to dst in chunks, checking ctx between reads so
// a canceled or expired context aborts the copy promptly. io.Copy is not
// used because its WriterTo shortcut would bypass these checks for sources
// that implement io.WriterTo (e.g. strings.Reader, bytes.Reader, os.File).
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var n int64
	for {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		m, rerr := src.Read(buf)
		if m > 0 {
			written, werr := dst.Write(buf[:m])
			n += int64(written)
			if werr != nil {
				return n, werr
			}
			if written != m {
				return n, io.ErrShortWrite
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				return n, nil
			}
			return n, rerr
		}
	}
}

// Remove removes the file with path name. It satisfies the WriteFS.Remove
// contract: a missing file yields an error that satisfies
// errors.Is(err, fs.ErrNotExist) (the underlying os.Remove error), while
// removing the top-level directory (".") is rejected without touching the
// storage root, though with a backend-specific error (not fs.ErrNotExist).
func (fsys *FS) Remove(ctx context.Context, name string) error {
	fullPath, err := fsys.osPath(name)
	if err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	if name == "." {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  errors.New("cannot remove top-level directory"),
		}
	}
	if err := ctx.Err(); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	if err := os.Remove(fullPath); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	return nil
}

func (fsys *FS) RemoveAll(ctx context.Context, name string) error {
	fullPath, err := fsys.osPath(name)
	if err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	if name == "." {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  errors.New("cannot remove top-level directory"),
		}
	}
	if err := ctx.Err(); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	// The trailing slash marks fullPath as a directory entry, but it is not
	// what provides symlink safety: os.RemoveAll strips trailing separators
	// and never follows symlinks, so a symlink at this path is removed as a
	// link and its target — even one outside the storage root — is never
	// touched. Keep that invariant if this call is ever rewritten (e.g. a
	// custom recursive walk must not follow the link).
	if err := os.RemoveAll(fullPath + "/"); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	return nil
}

func (fsys *FS) osPath(name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", fs.ErrInvalid
	}
	return filepath.Join(fsys.path, filepath.FromSlash(name)), nil
}
