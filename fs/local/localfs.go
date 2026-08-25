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

	"github.com/rogpeppe/go-internal/robustio"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

const (
	dirPerm = 0755
	// tempPerm is the creation mode for the temp file behind Write's
	// atomic-write sequence. It is subject to the process umask, so a new
	// file lands at 0666 &^ umask (typically 0644), matching os.Create. An
	// existing target's mode is chmod'd onto the temp file instead.
	tempPerm = 0666
)

type FS struct {
	ocflfs.DirEntriesFS
	// path is os-specific path to a directory
	path string
}

var _ ocflfs.WriteFS = (*FS)(nil)
var _ ocflfs.DirEntriesFS = (*FS)(nil)

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

// Write writes the contents of src to the file named name, creating any
// missing parent directories, and returns the number of bytes written.
//
// The replacement happens in one step: src is copied to a unique temporary
// file (".<base>.tmp-<random>") in the target's own directory, synced, and
// then renamed over name once the copy is complete — an atomic replace on
// POSIX and, via MoveFileEx(MOVEFILE_REPLACE_EXISTING), on Windows. A reader
// of name therefore observes either the previous file or the new file in
// full, never a truncated or partially written one. A failed copy, a canceled
// context or a crash before the rename leaves the previous contents
// untouched, and creates nothing at name when it did not exist. Every path
// before the rename removes the temporary file (best-effort), so failed
// writes do not leak one; a crash can leave one behind, where it is visible
// as an ordinary dot-file.
//
// If name already exists as a regular file, its permissions are carried onto
// the replacement exactly. Otherwise the new file is created with mode 0666
// subject to the process umask, matching os.Create. A symlink at name is
// replaced by the link entry being renamed over: the referent is not written
// through and neither its mode nor the link's own 0777 is copied onto the new
// regular file.
//
// ctx is honored between reads from src: a canceled or expired context aborts
// the copy and is reported as itself, so callers can match context.Canceled
// and context.DeadlineExceeded. Cancellation is not observed during the final
// rename, which is one syscall from committing an already-complete copy.
//
// On any failure before the rename the returned count is 0: no bytes reached
// name.
func (fsys *FS) Write(ctx context.Context, name string, src io.Reader) (int64, error) {
	fullPath, err := fsys.osPath(name)
	if err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	// "." names the storage root, never a file. fs.ValidPath accepts it, and
	// without this guard the rejection would come from the OS refusing to
	// open a directory for writing — an EISDIR that callers cannot match
	// against fs.ErrInvalid the way they match every other bad name.
	if name == "." {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  fs.ErrInvalid,
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
	// file: the temp file is chmod'd to the exact mode, and chmod is not
	// umask-masked, so the mode is copied rather than approximated. The
	// chmod happens after the (umask-affected) create, so it always wins.
	//
	// The target is examined with Lstat and only a regular file contributes
	// a mode. For a symlink neither mode on hand describes the file being
	// created: Stat would give the referent's, which belongs to a file that
	// is not being written, and Lstat gives the link's own, which is 0777 on
	// POSIX and would publish a world-writable regular file. IsRegular
	// rejects both, along with directories and devices, in one condition.
	//
	// havePerm, rather than a preserveMode != 0 sentinel, distinguishes "no
	// mode to preserve" from a target whose mode legitimately is 0000. Lstat
	// errors are tolerated: a missing target is simply a new file at the
	// default temp mode.
	var (
		preserveMode fs.FileMode
		havePerm     bool
	)
	if info, err := os.Lstat(fullPath); err == nil && info.Mode().IsRegular() {
		preserveMode, havePerm = info.Mode().Perm(), true
	}
	// The temp file goes in the target's own directory so the final rename
	// is within one filesystem, where it is an atomic replace. O_EXCL at
	// creation means it can never clobber an existing file, even if another
	// writer races in the same directory.
	tmpPath, tmp, err := createTempFile(parent, fullPath)
	if err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	// Until the rename commits, every return path closes and removes the
	// temp file (best-effort), so a failed or canceled write leaves no litter.
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
	n, err := io.Copy(tmp, ctxReader{ctx: ctx, src: src})
	if err != nil {
		// A copy that stopped because the context ended reports the context
		// error, not whatever the reader or writer said on the way out, so
		// callers can match context.Canceled / context.DeadlineExceeded.
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	// Sync before the rename makes the file's contents durable ahead of the
	// name that publishes them: a crash cannot leave name pointing at a
	// zero-length or partially flushed file.
	if err := tmp.Sync(); err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	// Close must precede the rename: Windows cannot move a file that is
	// still open.
	if err := tmp.Close(); err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	// The atomic swap. robustio.Rename is os.Rename plus a bounded retry
	// (~2s, randomized backoff) on the transient Windows errors a virus
	// scanner or the search indexer causes by holding a handle on the
	// just-closed temp file: ERROR_ACCESS_DENIED, ERROR_SHARING_VIOLATION,
	// ERROR_FILE_NOT_FOUND. On darwin it retries ENOENT; on every other
	// platform it is a plain call through to os.Rename.
	//
	// os.Rename is the right primitive on Windows too, contrary to a common
	// belief that it cannot replace an existing file there: since Go 1.16 it
	// is MoveFileEx(MOVEFILE_REPLACE_EXISTING) with fixLongPath applied, an
	// atomic replace that also handles paths past MAX_PATH — which long OCFL
	// content paths reach. This module requires Go 1.25 (see go.mod), well
	// past that.
	//
	// The retry sleeps without consulting ctx, so a canceled write can block
	// here for up to ~2s. That is deliberate: the copy is already complete
	// and the write is one syscall from committing, so finishing beats
	// abandoning it.
	if err := robustio.Rename(tmpPath, fullPath); err != nil {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  err,
		}
	}
	committed = true
	// Best-effort fsync of the containing directory persists the rename
	// itself across a crash — without it a committed inventory can survive
	// as content while its name does not. Failures are ignored: the write
	// has already succeeded, and not every filesystem supports fsync on a
	// directory.
	if dir, err := os.Open(parent); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return n, nil
}

// ctxReader makes a copy from src cancellable by checking ctx before each
// read.
//
// It deliberately exposes only Read. Sources like *strings.Reader,
// *bytes.Reader and *os.File implement io.WriterTo, which io.Copy consults
// before anything else; a wrapper that forwarded it would hand the copy a
// fast path that never calls Read and so never sees the context.
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

// tempFileName returns the name of a temporary file for target, of the form
// ".<base>.tmp-<random>". The leading dot keeps it out of ordinary listings
// and the random suffix (enforced with O_EXCL at creation) keeps concurrent
// writers from colliding.
//
// base is truncated as needed to keep the whole name within NAME_MAX (255
// bytes), which long OCFL content names reach. The truncation backs off to a
// UTF-8 rune boundary: slicing raw bytes at the limit can split a multibyte
// rune, producing an invalid name that strict filesystems reject with a
// confusing error.
func tempFileName(target string) string {
	base := filepath.Base(target)
	const reserved = len(".") + len(".tmp-") + 16 // leading dot, suffix, hex random
	if max := 255 - reserved; len(base) > max {
		// Walk back from the byte limit to the nearest byte that starts a
		// rune, so the slice never ends mid-sequence. base[max] is in range
		// here (len(base) > max), and the cut > 0 guard stops the scan at
		// the front of the string, so base[:cut] is valid UTF-8 for any
		// valid UTF-8 input.
		cut := max
		for cut > 0 && !utf8.RuneStart(base[cut]) {
			cut--
		}
		base = base[:cut]
	}
	return "." + base + ".tmp-" + fmt.Sprintf("%016x", rand.Uint64())
}

// createTempFile creates a new file in dir named after target with
// O_CREATE|O_EXCL|O_WRONLY, so it can never clobber an existing file. A name
// collision (practically impossible) is retried with a fresh random suffix.
// The file is created at tempPerm, subject to the process umask.
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

func (fsys *FS) Remove(ctx context.Context, name string) error {
	fullPath, err := fsys.osPath(name)
	if err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  err,
		}
	}
	// "." names the storage root, never a file, so removing it is a bad
	// name rather than a failed removal — the same rejection Write gives it.
	if name == "." {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  fs.ErrInvalid,
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
