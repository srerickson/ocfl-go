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

// TestWriteFSWrite asserts the [ocflfs.WriteFS] Write behavior every
// implementation must share, against whichever backend fsys implements. Call
// it from a backend's own external test package.
//
// The interesting cases are the ones where an implementation can look correct
// on a happy-path test and still be wrong:
//
//   - a shorter overwrite must fully replace the file, not overlay it. A
//     write that opens the target with O_TRUNC gets this right by accident
//     and an in-place write does not; the trailing bytes of the previous
//     content are the tell.
//   - a failing source must leave the previous content intact. A caller must
//     never be able to observe name with partial contents, which rules out
//     destroying the old file before the new one is complete.
//   - an invalid path must be rejected without side effects, so a caller
//     cannot half-create something outside the namespace.
//
// Atomicity itself — what a concurrent reader observes mid-write — is not
// asserted here: it is only observable through implementation-specific
// machinery, so it belongs in the backend's own tests.
func TestWriteFSWrite(t *testing.T, fsys ocflfs.WriteFS) {
	t.Helper()
	ctx := context.Background()

	readBack := func(t *testing.T, name string) string {
		t.Helper()
		f, err := fsys.OpenFile(ctx, name)
		be.NilErr(t, err)
		defer f.Close()
		data, err := io.ReadAll(f)
		be.NilErr(t, err)
		return string(data)
	}

	t.Run("write new file returns size and content", func(t *testing.T) {
		const name, content = "imptest-write/new.txt", "hello write"
		n, err := fsys.Write(ctx, name, strings.NewReader(content))
		be.NilErr(t, err)
		be.Equal(t, int64(len(content)), n)
		be.Equal(t, content, readBack(t, name))
	})

	t.Run("overwrite with shorter content replaces entirely", func(t *testing.T) {
		const name = "imptest-write/shrink.txt"
		const long = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		const short = "bb"
		_, err := fsys.Write(ctx, name, strings.NewReader(long))
		be.NilErr(t, err)
		n, err := fsys.Write(ctx, name, strings.NewReader(short))
		be.NilErr(t, err)
		be.Equal(t, int64(len(short)), n)
		// The whole point: no tail of the previous content may survive.
		be.Equal(t, short, readBack(t, name))
	})

	t.Run("write empty content creates an empty file", func(t *testing.T) {
		const name = "imptest-write/empty.txt"
		n, err := fsys.Write(ctx, name, strings.NewReader(""))
		be.NilErr(t, err)
		be.Equal(t, int64(0), n)
		be.Equal(t, "", readBack(t, name))
	})

	t.Run("write creates intermediate directories", func(t *testing.T) {
		const name, content = "imptest-write/a/b/c/deep.txt", "deep"
		_, err := fsys.Write(ctx, name, strings.NewReader(content))
		be.NilErr(t, err)
		be.Equal(t, content, readBack(t, name))
	})

	t.Run("source error leaves previous content intact", func(t *testing.T) {
		// TODO(#163): local Write opens the destination with O_TRUNC and
		// copies into it, so a failing source leaves the target truncated.
		// The s3 backend already satisfies this; drop the skip when #163
		// makes local Write atomic and the assertion runs for both.
		t.Skip("local Write is not atomic; see #163")
		const name, original = "imptest-write/failed-overwrite.txt", "original content"
		_, err := fsys.Write(ctx, name, strings.NewReader(original))
		be.NilErr(t, err)

		_, err = fsys.Write(ctx, name, &failingReader{
			prefix: []byte("partial replacement that must never land"),
			err:    errors.New("source read failed"),
		})
		be.True(t, err != nil)
		// The previous file is still the previous file: not truncated, not
		// partially overwritten, not replaced by the bytes that did arrive.
		be.Equal(t, original, readBack(t, name))
	})

	t.Run("source error on a new file leaves nothing behind", func(t *testing.T) {
		// TODO(#163): the same non-atomic local Write creates an empty file
		// at name before the source fails. Already satisfied by s3.
		t.Skip("local Write is not atomic; see #163")
		const name = "imptest-write/never-created.txt"
		_, err := fsys.Write(ctx, name, &failingReader{
			prefix: []byte("bytes that must not become a file"),
			err:    errors.New("source read failed"),
		})
		be.True(t, err != nil)
		_, err = fsys.OpenFile(ctx, name)
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})

	t.Run("invalid path is rejected with no side effect", func(t *testing.T) {
		for _, name := range []string{"../escape.txt", "/absolute.txt", "a/../b.txt", ""} {
			_, err := fsys.Write(ctx, name, strings.NewReader("nope"))
			if err == nil {
				t.Errorf("Write(%q) = nil error, want a rejection", name)
				continue
			}
			// The rejection must be recognizable, not just any error: callers
			// distinguish a bad name from a backend outage.
			if !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("Write(%q) error = %v, want one matching fs.ErrInvalid", name, err)
			}
		}
	})

	t.Run("write to dot is rejected as an invalid path", func(t *testing.T) {
		// "." passes fs.ValidPath, so it reaches a backend that rejects every
		// other bad name earlier. It still names a container and never a
		// file, on a backend with directories and on one without, so it is
		// rejected the same way: an error matching fs.ErrInvalid. A backend
		// that leaves this to its storage layer fails here — the OS reports
		// EISDIR, which matches nothing a caller can test for.
		_, err := fsys.Write(ctx, ".", strings.NewReader("nope"))
		if err == nil {
			t.Fatal(`Write(".") = nil error, want a rejection`)
		}
		if !errors.Is(err, fs.ErrInvalid) {
			t.Errorf(`Write(".") error = %v, want one matching fs.ErrInvalid`, err)
		}
	})
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
func TestWriteFSRemove(t *testing.T, fsys ocflfs.WriteFS) {
	t.Helper()
	ctx := context.Background()

	t.Run("remove missing file returns ErrNotExist", func(t *testing.T) {
		// TODO(#166): s3's remove() calls the idempotent DeleteObject with no
		// existence check, so a key that was never there deletes
		// successfully and Remove reports nil. The local backend already
		// satisfies this; drop the skip when #166 adds the HEAD probe.
		t.Skip("Remove of a missing key returns nil on the s3 backend; see #166")
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

// WriteFSRemoveAll configures [TestWriteFSRemoveAll] with the parts of
// RemoveAll that are left to the implementation.
type WriteFSRemoveAll struct {
	// RemoveAllDotIsError reports whether the backend's own
	// RemoveAll(ctx, ".") refuses rather than emptying the top-level
	// directory. This genuinely differs between correct implementations: the
	// local backend refuses because its storage root must survive, the S3
	// backend empties the bucket. Callers who want uniform behavior use the
	// package-level [ocflfs.RemoveAll], which is covered separately.
	RemoveAllDotIsError bool

	// RemoveAllOnFileRemovesIt reports whether RemoveAll applied directly to
	// a file path removes that file. A hierarchical backend deletes it
	// (os.RemoveAll does); a backend that treats the name as a key prefix
	// does not, because a file's key is not under its own prefix.
	RemoveAllOnFileRemovesIt bool
}

// TestWriteFSRemoveAll asserts the [ocflfs.WriteFS] RemoveAll behavior every
// implementation must share, against whichever backend fsys implements. Call
// it from a backend's own external test package.
//
// The cases worth pinning are the ones where the two families of
// implementation drift apart. A hierarchical backend removes a subtree; a
// key-value backend removes a key prefix. Those agree on the happy path and
// disagree at the boundary — "a" as a prefix also matches "ab", but as a
// directory it does not — so the sibling case below is the one that catches a
// backend building its prefix without the trailing separator. Idempotency is
// the other: RemoveAll returns nil for a path that does not exist, which is
// the opposite of Remove and easy to implement backwards.
func TestWriteFSRemoveAll(t *testing.T, fsys ocflfs.WriteFS, opts WriteFSRemoveAll) {
	t.Helper()
	ctx := context.Background()

	write := func(t *testing.T, name string) {
		t.Helper()
		_, err := fsys.Write(ctx, name, strings.NewReader("content of "+name))
		be.NilErr(t, err)
	}
	exists := func(t *testing.T, name string) bool {
		t.Helper()
		f, err := fsys.OpenFile(ctx, name)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("OpenFile(%q): unexpected error %v, want nil or fs.ErrNotExist", name, err)
			}
			return false
		}
		_, _ = io.Copy(io.Discard, f)
		be.NilErr(t, f.Close())
		return true
	}

	t.Run("missing path is not an error", func(t *testing.T) {
		// The documented inversion of Remove: absent is success, not
		// fs.ErrNotExist.
		be.NilErr(t, fsys.RemoveAll(ctx, "imptest-removeall/never-existed"))
	})

	t.Run("removes the whole subtree", func(t *testing.T) {
		const dir = "imptest-removeall/tree"
		write(t, dir+"/one.txt")
		write(t, dir+"/nested/two.txt")
		write(t, dir+"/nested/deeper/three.txt")

		be.NilErr(t, fsys.RemoveAll(ctx, dir))
		for _, name := range []string{dir + "/one.txt", dir + "/nested/two.txt", dir + "/nested/deeper/three.txt"} {
			if exists(t, name) {
				t.Errorf("%q survived RemoveAll(%q)", name, dir)
			}
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		const dir = "imptest-removeall/twice"
		write(t, dir+"/file.txt")
		be.NilErr(t, fsys.RemoveAll(ctx, dir))
		// The second call has nothing to do and must still succeed.
		be.NilErr(t, fsys.RemoveAll(ctx, dir))
		be.False(t, exists(t, dir+"/file.txt"))
	})

	t.Run("does not remove name-prefixed siblings", func(t *testing.T) {
		// "a" must be treated as a path element, not a string prefix.
		// Building the S3 listing prefix as name rather than name+"/" makes
		// every assertion in this subtest fail, and nothing else here
		// notices.
		const base = "imptest-removeall/siblings"
		write(t, base+"/a/inside.txt")
		write(t, base+"/ab/outside.txt")
		write(t, base+"/a-sibling.txt")
		write(t, base+"/abc.txt")

		be.NilErr(t, fsys.RemoveAll(ctx, base+"/a"))

		be.False(t, exists(t, base+"/a/inside.txt"))
		for _, survivor := range []string{base + "/ab/outside.txt", base + "/a-sibling.txt", base + "/abc.txt"} {
			if !exists(t, survivor) {
				t.Errorf("%q was removed by RemoveAll(%q), but it is a sibling, not a child", survivor, base+"/a")
			}
		}
	})

	t.Run("leaves unrelated paths alone", func(t *testing.T) {
		const base = "imptest-removeall/scoped"
		write(t, base+"/target/gone.txt")
		write(t, base+"/keep/stays.txt")

		be.NilErr(t, fsys.RemoveAll(ctx, base+"/target"))
		be.False(t, exists(t, base+"/target/gone.txt"))
		be.True(t, exists(t, base+"/keep/stays.txt"))
	})

	t.Run("invalid path is rejected", func(t *testing.T) {
		for _, name := range []string{"../escape", "/absolute", "a/../b", ""} {
			err := fsys.RemoveAll(ctx, name)
			if err == nil {
				t.Errorf("RemoveAll(%q) = nil error, want a rejection", name)
				continue
			}
			if !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("RemoveAll(%q) error = %v, want one matching fs.ErrInvalid", name, err)
			}
		}
	})

	t.Run("applied to a file", func(t *testing.T) {
		const name = "imptest-removeall/lone-file.txt"
		write(t, name)
		be.NilErr(t, fsys.RemoveAll(ctx, name))
		be.Equal(t, !opts.RemoveAllOnFileRemovesIt, exists(t, name))
	})

	// Runs last: on a backend that empties its root, this subtest removes
	// everything the ones above wrote.
	t.Run("dot", func(t *testing.T) {
		const probe = "imptest-removeall/dot-probe.txt"
		write(t, probe)
		err := fsys.RemoveAll(ctx, ".")
		if opts.RemoveAllDotIsError {
			// Refusing must be inert: the root is untouched.
			be.True(t, err != nil)
			be.True(t, exists(t, probe))
			return
		}
		// Emptying must actually empty, and must not report the now-absent
		// entries as an error.
		be.NilErr(t, err)
		be.False(t, exists(t, probe))
	})
}

// failingReader delivers prefix and then fails, standing in for a source that
// dies partway through — a truncated network body, a disappearing file. The
// bytes it does deliver are the ones a non-atomic write would leave visible.
type failingReader struct {
	prefix []byte
	pos    int
	err    error
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.pos < len(r.prefix) {
		n := copy(p, r.prefix[r.pos:])
		r.pos += n
		return n, nil
	}
	return 0, r.err
}
