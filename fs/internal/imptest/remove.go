package imptest

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// WriteFSRemove configures [TestWriteFSRemove]. Remove has no
// implementation-specific behavior left to configure, so the struct carries
// only the Skip fields for behavior a backend does not satisfy yet.
type WriteFSRemove struct {
	// SkipMissingIsNotExist, when non-empty, skips the missing-file subtest
	// using this string as the skip reason.
	SkipMissingIsNotExist string
}

// TestWriteFSRemove asserts the [ocflfs.WriteFS] Remove behavior every
// implementation must share, against whichever backend fsys implements. Call
// it from a backend's own external test package.
//
// The test covers:
//   - removing a missing file returns an error satisfying
//     errors.Is(err, fs.ErrNotExist), reported as a *fs.PathError with
//     Op "remove". S3 does not give this for free: DeleteObject is
//     idempotent and succeeds for a key that was never there;
//   - removing "." is rejected with fs.ErrInvalid, as a *fs.PathError naming
//     ".", and never touches the storage root: a file written before the call
//     must still be present and readable afterwards. "." names the storage
//     root and not a file on every backend, so it is a bad name rather than a
//     failed removal -- and in particular not fs.ErrNotExist, which would
//     tell a caller the root is absent when it is the one thing guaranteed to
//     exist.
func TestWriteFSRemove(t *testing.T, fsys ocflfs.WriteFS, opts WriteFSRemove) {
	t.Helper()
	ctx := context.Background()

	t.Run("remove missing file returns ErrNotExist", func(t *testing.T) {
		if opts.SkipMissingIsNotExist != "" {
			t.Skip(opts.SkipMissingIsNotExist)
		}
		const missing = "no-such-file.txt"
		err := fsys.Remove(ctx, missing)
		be.True(t, err != nil)
		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "remove", pathErr.Op)
		be.Equal(t, missing, pathErr.Path)
		// Every backend reports a missing file as fs.ErrNotExist, so callers
		// can detect it uniformly with errors.Is regardless of backend.
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})

	t.Run("remove dot errors and leaves root intact", func(t *testing.T) {
		const (
			probe   = "imptest-remove/root-probe.txt"
			payload = "probe payload"
		)
		_, err := fsys.Write(ctx, probe, strings.NewReader(payload))
		be.NilErr(t, err)
		err = fsys.Remove(ctx, ".")
		be.True(t, err != nil)
		var pathErr *fs.PathError
		be.True(t, errors.As(err, &pathErr))
		be.Equal(t, "remove", pathErr.Op)
		be.Equal(t, ".", pathErr.Path)
		// One classification for every backend, so a caller can recognize
		// the refusal without knowing which storage it is talking to, and
		// with the same handling it gives every other bad name.
		be.True(t, errors.Is(err, fs.ErrInvalid))
		be.False(t, errors.Is(err, fs.ErrNotExist))
		// The storage root must be unaffected: the probe file written before
		// Remove(".") must still be present and readable.
		file, err := fsys.OpenFile(ctx, probe)
		be.NilErr(t, err)
		data, err := io.ReadAll(file)
		be.NilErr(t, err)
		be.NilErr(t, file.Close())
		be.Equal(t, payload, string(data))
	})

	t.Run("removes an existing file", func(t *testing.T) {
		const name = "imptest-remove/goes-away.txt"
		_, err := fsys.Write(ctx, name, strings.NewReader("x"))
		be.NilErr(t, err)
		be.NilErr(t, fsys.Remove(ctx, name))
		// Gone means gone: the file must read back as missing, not merely
		// have been named in a delete request.
		_, err = fsys.OpenFile(ctx, name)
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})

	t.Run("invalid path is rejected", func(t *testing.T) {
		for _, name := range []string{"../escape.txt", "/absolute.txt", "a/../b.txt", ""} {
			err := fsys.Remove(ctx, name)
			if err == nil {
				t.Errorf("Remove(%q) = nil error, want a rejection", name)
				continue
			}
			if !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("Remove(%q) error = %v, want one matching fs.ErrInvalid", name, err)
			}
		}
	})
}
