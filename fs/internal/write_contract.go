package internal

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

// WriteFSWriteContract configures [TestWriteFSWriteContract] with the parts of
// the [ocflfs.WriteFS] Write contract a backend is allowed to decide for
// itself, plus the Skip fields for cases a backend does not honor yet.
type WriteFSWriteContract struct {
	// WriteDotIsError reports whether Write(ctx, ".", …) must fail. Every
	// backend where "." names a directory rejects it; a backend with no
	// directories at all may treat it as an ordinary key.
	WriteDotIsError bool

	// SkipFailedSourceKeepsFile, when non-empty, skips the two subtests that
	// require a failing source to leave the target as it was, using this
	// string as the skip reason.
	SkipFailedSourceKeepsFile string
}

// TestWriteFSWriteContract asserts the [ocflfs.WriteFS] Write behavior that
// all backends share, against whichever backend fsys implements. Call it from
// a backend's own external test package.
//
// The interesting cases are the ones where a backend can look correct on a
// happy-path test and still be wrong:
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
// asserted here: it is only observable through backend-specific machinery, so
// it belongs in the backend's own tests.
func TestWriteFSWriteContract(t *testing.T, fsys ocflfs.WriteFS, contract WriteFSWriteContract) {
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
		const name, content = "write-contract/new.txt", "hello contract"
		n, err := fsys.Write(ctx, name, strings.NewReader(content))
		be.NilErr(t, err)
		be.Equal(t, int64(len(content)), n)
		be.Equal(t, content, readBack(t, name))
	})

	t.Run("overwrite with shorter content replaces entirely", func(t *testing.T) {
		const name = "write-contract/shrink.txt"
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
		const name = "write-contract/empty.txt"
		n, err := fsys.Write(ctx, name, strings.NewReader(""))
		be.NilErr(t, err)
		be.Equal(t, int64(0), n)
		be.Equal(t, "", readBack(t, name))
	})

	t.Run("write creates intermediate directories", func(t *testing.T) {
		const name, content = "write-contract/a/b/c/deep.txt", "deep"
		_, err := fsys.Write(ctx, name, strings.NewReader(content))
		be.NilErr(t, err)
		be.Equal(t, content, readBack(t, name))
	})

	t.Run("source error leaves previous content intact", func(t *testing.T) {
		if contract.SkipFailedSourceKeepsFile != "" {
			t.Skip(contract.SkipFailedSourceKeepsFile)
		}
		const name, original = "write-contract/failed-overwrite.txt", "original content"
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
		if contract.SkipFailedSourceKeepsFile != "" {
			t.Skip(contract.SkipFailedSourceKeepsFile)
		}
		const name = "write-contract/never-created.txt"
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

	if contract.WriteDotIsError {
		t.Run("write to dot is rejected", func(t *testing.T) {
			_, err := fsys.Write(ctx, ".", strings.NewReader("nope"))
			be.True(t, err != nil)
		})
	}
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
