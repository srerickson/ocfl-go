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

// WriteFSRemoveAllContract configures [TestWriteFSRemoveAllContract] with the
// parts of the [ocflfs.WriteFS] RemoveAll contract that are left to the
// backend.
type WriteFSRemoveAllContract struct {
	// RemoveAllDotIsError reports whether the backend's own
	// RemoveAll(ctx, ".") refuses rather than emptying the top-level
	// directory. This is genuinely backend-dependent: the local backend
	// refuses because its storage root must survive, the S3 backend empties
	// the bucket. Callers who want uniform behavior use the package-level
	// [ocflfs.RemoveAll], which is covered separately.
	RemoveAllDotIsError bool

	// RemoveAllOnFileRemovesIt reports whether RemoveAll applied directly to
	// a file path removes that file. A hierarchical backend deletes it
	// (os.RemoveAll does); a backend that treats the name as a key prefix
	// does not, because a file's key is not under its own prefix.
	RemoveAllOnFileRemovesIt bool
}

// TestWriteFSRemoveAllContract asserts the [ocflfs.WriteFS] RemoveAll behavior
// that all backends share, against whichever backend fsys implements. Call it
// from a backend's own external test package.
//
// The cases worth pinning across backends are the ones where the two families
// of implementation drift apart. A hierarchical backend removes a subtree; a
// key-value backend removes a key prefix. Those agree on the happy path and
// disagree at the boundary — "a" as a prefix also matches "ab", but as a
// directory it does not — so the sibling case below is the one that catches a
// backend building its prefix without the trailing separator. Idempotency is
// the other: RemoveAll returns nil for a path that does not exist, which is
// the opposite of Remove and easy to implement backwards.
func TestWriteFSRemoveAllContract(t *testing.T, fsys ocflfs.WriteFS, contract WriteFSRemoveAllContract) {
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
		be.NilErr(t, fsys.RemoveAll(ctx, "removeall-contract/never-existed"))
	})

	t.Run("removes the whole subtree", func(t *testing.T) {
		const dir = "removeall-contract/tree"
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
		const dir = "removeall-contract/twice"
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
		const base = "removeall-contract/siblings"
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
		const base = "removeall-contract/scoped"
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
		const name = "removeall-contract/lone-file.txt"
		write(t, name)
		be.NilErr(t, fsys.RemoveAll(ctx, name))
		be.Equal(t, !contract.RemoveAllOnFileRemovesIt, exists(t, name))
	})

	// Runs last: on a backend that empties its root, this subtest removes
	// everything the ones above wrote.
	t.Run("dot", func(t *testing.T) {
		const probe = "removeall-contract/dot-probe.txt"
		write(t, probe)
		err := fsys.RemoveAll(ctx, ".")
		if contract.RemoveAllDotIsError {
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
