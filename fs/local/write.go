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

// Write writes the contents of src to the file named name, creating the
// file and any missing parent directories, and returns the number of
// bytes written. The contents are copied to a unique temporary file
// (.<base>.tmp-<random>) in the target's own directory, synced to disk,
// and only the complete temp file is then moved over name, so readers
// never observe a partial file: a crash, a canceled context, or any
// other failure before the move leaves either the old file or no file at
// name, never a truncated one. On POSIX the move is an atomic os.Rename
// replacement. On Windows, os.Rename cannot replace an existing file, so
// the move goes through renameReplaceWindows (MoveFileEx with
// MOVEFILE_REPLACE_EXISTING, falling back to Remove+Rename); that
// replacement is best-effort, not atomic — a failure between the Remove
// and the Rename can leave no file at name (see rename_windows.go). On
// any failure before the move the temporary file is removed
// (best-effort), so failed writes do not leak temp files. If name
// already exists, its permissions are preserved on the replacement;
// otherwise the new file is created with mode 0666 subject to the
// process umask. If name is a symlink, the move replaces the link entry
// itself — the referent is untouched, and the replacement inherits the
// link's own mode, never the referent's.
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
	// Preserve the target's permissions when it already exists as a regular
	// file: the temp file is chmod'd to the exact mode (chmod is not
	// umask-masked). For a new file, the temp file is left at
	// tempPerm &^ umask, matching plain os.Create semantics.
	//
	// The target is examined with Lstat, which does not follow symlinks, and
	// a symlink target is deliberately treated like a new file. The rename
	// below replaces the link entry itself with a regular file, so neither
	// mode on hand is the right one to keep: the referent's mode belongs to
	// a file that is not being written, and the link's own mode is 0777 on
	// POSIX, which would publish a world-writable regular file. Falling back
	// to the default temp mode is the only safe choice.
	//
	// havePerm, rather than a preserveMode != 0 sentinel, distinguishes "no
	// mode to preserve" from a target whose mode legitimately is 0000.
	// Lstat errors are tolerated: a missing target simply means a new file
	// created with the default temp mode.
	var (
		preserveMode fs.FileMode
		havePerm     bool
	)
	if info, err := os.Lstat(fullPath); err == nil && info.Mode()&fs.ModeSymlink == 0 {
		preserveMode, havePerm = info.Mode().Perm(), true
	}
	// Write to a unique temp file in the same directory as the target: the
	// final move is then on the same filesystem on every platform — a
	// requirement for os.Rename on POSIX and for the Windows rename helpers —
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
	if havePerm {
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
	// Final swap: readers of fullPath see either the old file or the new
	// complete file, never a partial one. renameReplace is os.Rename on
	// POSIX (an atomic replacement); on Windows it delegates to the
	// MoveFileEx/Remove+Rename helper (best-effort — see rename_windows.go).
	if err := renameReplace(tmpPath, fullPath); err != nil {
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
	// limit, not max: shadowing the builtin here would be legal but reads
	// badly in a size calculation.
	if limit := 255 - reserved; len(base) > limit {
		// Walk back from the byte limit to the nearest byte that starts a
		// rune, so the slice never ends in the middle of a UTF-8 sequence.
		// base[limit] is always in range here (len(base) > limit), and the
		// cut > 0 guard stops the scan at the front of the string, so
		// base[:cut] is a valid UTF-8 prefix for any valid UTF-8 input.
		cut := limit
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

// ctxReader wraps a reader so that a canceled or expired context aborts the
// read. It deliberately exposes nothing but Read: hiding the underlying
// reader's io.WriterTo (strings.Reader, bytes.Reader, *os.File) and its
// concrete type is what keeps io.Copy and os.File.ReadFrom from taking a
// shortcut that would bypass the context check entirely.
type ctxReader struct {
	ctx context.Context
	src io.Reader
}

func (r ctxReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.src.Read(p)
}

// copyWithContext copies src to dst, checking ctx before each read so a
// canceled or expired context aborts the copy promptly.
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, ctxReader{ctx: ctx, src: src})
}
