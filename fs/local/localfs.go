package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"math/rand/v2"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

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

// FS is a storage backend rooted in a local directory. Every operation goes
// through a single [os.Root] opened for that directory, so a name can never
// resolve to a file outside it: os.Root walks a name component by component
// with the openat family, refusing to follow a symlink whose target leaves the
// root. That holds for reads as well as writes — the difference from
// [os.DirFS], whose documentation is explicit that it does follow symlinks out
// of the directory it was given.
//
// One consequence is stricter than "stays inside the root": a symlink whose
// target is an absolute path is refused even when that path names a file
// inside the root, because os.Root treats any absolute target as leaving it.
// Relative symlinks, chains included, resolve normally so long as every step
// stays inside.
type FS struct {
	// path is the os-specific path to the root directory. It is kept for
	// Root() and is never used to build a path handed to the OS.
	path string
	// root scopes every operation to path. All file access goes through it.
	root *os.Root
}

var _ ocflfs.WriteFS = (*FS)(nil)
var _ ocflfs.DirEntriesFS = (*FS)(nil)

// NewFS returns an FS for the directory at path, which must exist and be a
// directory: the [os.Root] opened here holds a descriptor on it, so a missing
// or non-directory path fails here rather than at the first read or write.
//
// The returned FS owns that descriptor. Callers that create many short-lived
// FS values should [FS.Close] them; the runtime finalizer os.Root installs
// reclaims a forgotten one at GC, exactly as it does for an [os.File].
func NewFS(path string) (*FS, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("new backend: %w", err)
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return nil, fmt.Errorf("new backend: %w", err)
	}
	return &FS{path: abs, root: root}, nil
}

// MustNewFS returns a new FS for path. It panics if the FS cannot be created
// -- most usefully in tests, where the path is a t.TempDir().
func MustNewFS(path string) *FS {
	fsys, err := NewFS(path)
	if err != nil {
		panic(err)
	}
	return fsys
}

// Close releases the descriptor held on the root directory. Operations on a
// closed FS fail; Close is safe to call more than once.
func (fsys *FS) Close() error {
	return fsys.root.Close()
}

func (fsys *FS) Root() string {
	return fsys.path
}

// OpenFile opens the named file for reading. name is resolved inside the
// storage root: a symlink pointing out of the root is an error rather than a
// door out of it.
func (fsys *FS) OpenFile(ctx context.Context, name string) (fs.File, error) {
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
	f, err := fsys.root.Open(name)
	if err != nil {
		// Root.Open already reports Path as the name it was given, relative
		// to the root, so nothing needs rewriting to keep the storage root
		// out of the error. Only Op differs ("openat"), which is normalized
		// here to the method's own name.
		var pathErr *fs.PathError
		if errors.As(err, &pathErr) {
			pathErr.Op = "openfile"
		}
		return nil, err
	}
	return f, nil
}

// DirEntries implements [ocflfs.DirEntriesFS], yielding the entries in the
// directory named name in sorted order.
func (fsys *FS) DirEntries(ctx context.Context, name string) iter.Seq2[fs.DirEntry, error] {
	return func(yield func(fs.DirEntry, error) bool) {
		if !fs.ValidPath(name) {
			yield(nil, &fs.PathError{
				Op:   "readdir",
				Path: name,
				Err:  fs.ErrInvalid,
			})
			return
		}
		dir, err := fsys.root.Open(name)
		if err != nil {
			var pathErr *fs.PathError
			if errors.As(err, &pathErr) {
				pathErr.Op = "readdir"
			}
			yield(nil, err)
			return
		}
		defer dir.Close()
		// ReadDir on the open handle returns entries in directory order.
		// Sorting here is not optional: DirEntriesFS documents sorted output
		// and WalkFiles depends on it. The previous implementation got the
		// sort for free from fs.ReadDir, which sorts only when the FS does
		// not implement ReadDirFS -- and os.DirFS does, sorting internally.
		// Reading the handle directly skips both, so it is done explicitly.
		entries, err := dir.ReadDir(-1)
		slices.SortFunc(entries, func(a, b fs.DirEntry) int {
			return strings.Compare(a.Name(), b.Name())
		})
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				yield(nil, &fs.PathError{
					Op:   "readdir",
					Path: name,
					Err:  err,
				})
				return
			}
			if !yield(entry, nil) {
				return
			}
		}
		// A partial read yields what it got and then the error, matching
		// fs.ReadDir.
		if err != nil {
			yield(nil, err)
		}
	}
}

// Write writes the contents of src to the file named name, creating any
// missing parent directories, and returns the number of bytes written.
//
// The replacement happens in one step: src is copied to a unique temporary
// file (".<base>.tmp-<random>") in the target's own directory, synced, and
// then renamed over name once the copy is complete — an atomic replace on
// POSIX and, via the replacing rename os.Root issues, on Windows. A reader
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
// Every component of name is resolved inside the storage root, so a symlink
// anywhere in the name — final component or intermediate directory — cannot
// place the new file outside it.
//
// ctx is honored between reads from src: a canceled or expired context aborts
// the copy and is reported as itself, so callers can match context.Canceled
// and context.DeadlineExceeded. Cancellation is not observed during the final
// rename, which is one syscall from committing an already-complete copy.
//
// On any failure before the rename the returned count is 0: no bytes reached
// name.
func (fsys *FS) Write(ctx context.Context, name string, src io.Reader) (int64, error) {
	if !fs.ValidPath(name) {
		return 0, &fs.PathError{
			Op:   "write",
			Path: name,
			Err:  fs.ErrInvalid,
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
	// Names are slash-separated fs.ValidPath names all the way down: os.Root
	// takes them in that form on every platform, so nothing here converts to
	// or from an OS-specific path.
	parent := path.Dir(name)
	// Errors from here on keep the *fs.PathError os.Root reports, nested
	// inside this method's own, rather than routing through unwrapPathError
	// the way Remove and RemoveAll do. There the inner error names the same
	// name the caller passed and is pure redundancy; here it names a
	// different path worth keeping -- mkdirat reports the component that
	// actually failed, and the temp file's create, chmod, sync and close
	// report the temp file. Neither names the storage root, and errors.Is
	// and errors.As see through either shape.
	if err := mkdirAll(fsys.root, parent); err != nil {
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
	if info, err := fsys.root.Lstat(name); err == nil && info.Mode().IsRegular() {
		preserveMode, havePerm = info.Mode().Perm(), true
	}
	// The temp file goes in the target's own directory so the final rename
	// is within one filesystem, where it is an atomic replace. O_EXCL at
	// creation means it can never clobber an existing file, even if another
	// writer races in the same directory.
	tmpName, tmp, err := createTempFile(fsys.root, parent, name)
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
			_ = tmp.Close()               // no-op if already closed
			_ = fsys.root.Remove(tmpName) // no-op if already renamed away
		}
	}()
	if havePerm {
		// Chmod on the open handle rather than Root.Chmod by name: this is
		// fchmod on the file just created, which sidesteps the race os.Root's
		// documentation warns about for Root.Chmod, where a name swapped to a
		// symlink mid-operation can direct the chmod at the link's target.
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
	// The atomic swap, resolved component-by-component inside the root on
	// both sides, so neither the source nor the destination can be redirected
	// out of it by a symlink.
	//
	// Root.Rename is a replacing rename on Windows too: it is
	// NtSetInformationFile with FileRenameInformationEx and
	// FILE_RENAME_REPLACE_IF_EXISTS|FILE_RENAME_POSIX_SEMANTICS, falling back
	// to FILE_RENAME_INFORMATION{ReplaceIfExists: true} on FAT and pre-1709
	// NTFS. So the atomicity this method documents holds there as well, and
	// the POSIX semantics additionally allow replacing a destination that has
	// open handles, which MoveFileEx cannot. Long paths need no special
	// handling: os.Root never expresses a name as one full path string, and
	// the MAX_PATH bound in the rename structure applies only to the final
	// component, which tempFileName already caps at 255 bytes.
	//
	// The bounded retry this call used to carry (robustio.Rename, for the
	// transient ERROR_ACCESS_DENIED / ERROR_SHARING_VIOLATION a Windows
	// virus scanner causes by holding a handle, plus an ENOENT retry on
	// darwin) is deliberately gone rather than reimplemented. CI is Linux
	// plus a GOOS=windows vet, so the workaround could never be exercised;
	// POSIX rename semantics remove the destination-side half of the problem
	// outright; and the retry is reintroducible behind a build tag if a real
	// failure ever shows up. See #164.
	if err := fsys.root.Rename(tmpName, name); err != nil {
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
	if dir, err := fsys.root.Open(parent); err == nil {
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
	base := path.Base(target)
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

// mkdirAll creates the directory named dir inside root, along with any
// missing parents, and reports success when it already exists as a directory.
//
// Root.MkdirAll is not used, because it races with itself on intermediate
// components: for each parent it opens the component and, on ENOENT, calls
// mkdirat once, returning EEXIST as an error if another goroutine created
// that directory in between (go1.25 os/root_openat.go, openDirFunc). Two
// concurrent writes under a common new prefix -- ordinary for a stage
// commit -- then fail with "file exists". os.MkdirAll never had the problem
// because it stats on EEXIST, which is what this does.
//
// Each component is created through root, so resolution stays inside the
// storage root. An existing component that is not a directory, and one whose
// symlink resolves outside the root (root.Stat fails), both surface as the
// original EEXIST.
func mkdirAll(root *os.Root, dir string) error {
	if dir == "." {
		return nil
	}
	built := ""
	for part := range strings.SplitSeq(dir, "/") {
		if built == "" {
			built = part
		} else {
			built += "/" + part
		}
		err := root.Mkdir(built, dirPerm)
		if err == nil {
			continue
		}
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
		if info, statErr := root.Stat(built); statErr != nil || !info.IsDir() {
			return err
		}
	}
	return nil
}

// createTempFile creates a new file in dir (a name within root) named after
// target with O_CREATE|O_EXCL|O_WRONLY, so it can never clobber an existing
// file. A name collision (practically impossible) is retried with a fresh
// random suffix. The file is created at tempPerm, subject to the process
// umask.
func createTempFile(root *os.Root, dir, target string) (string, *os.File, error) {
	for range 10 {
		name := path.Join(dir, tempFileName(target))
		f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, tempPerm)
		if err == nil {
			return name, f, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", nil, err
		}
	}
	return "", nil, errors.New("unable to create a unique temp file")
}

// Remove removes the file named name. name is resolved inside the storage
// root, so a symlink in an intermediate component cannot direct the removal
// at a file outside it. A symlink at name itself is removed as the link entry
// it is; its referent is untouched.
func (fsys *FS) Remove(ctx context.Context, name string) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  fs.ErrInvalid,
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
	if err := fsys.root.Remove(name); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  unwrapPathError(err),
		}
	}
	return nil
}

// RemoveAll removes name and any children it contains. name is resolved
// inside the storage root, so a symlink in an intermediate component cannot
// direct the removal at a tree outside it. A symlink at name itself is
// removed as the link entry it is; the tree it points at is untouched.
func (fsys *FS) RemoveAll(ctx context.Context, name string) error {
	if !fs.ValidPath(name) {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	// The guard stays ahead of the Root call: Root.RemoveAll(".") reports a
	// bare EINVAL that does not satisfy errors.Is(err, fs.ErrInvalid), so the
	// refusal is spelled out here where callers can match it.
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
	if err := fsys.root.RemoveAll(name); err != nil {
		return &fs.PathError{
			Op:   "remove",
			Path: name,
			Err:  unwrapPathError(err),
		}
	}
	return nil
}

// unwrapPathError returns the underlying error of a *fs.PathError, so an
// error from os.Root -- which reports its own Op and Path -- can be re-wrapped
// with this package's without the two nesting.
func unwrapPathError(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}
