package imptest

import (
	"context"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/carlmjohnson/be"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// TestDirEntries asserts the [ocflfs.DirEntriesFS] behavior every
// implementation must share, against whichever backend fsys implements.
//
// DirEntries is where the two backend families are most tempted to diverge,
// because S3 has no directories: its listing is flat, and the
// directory-shaped answer has to be reconstructed from key prefixes. Three
// properties keep the two interchangeable for callers, and each is a distinct
// way that reconstruction goes wrong:
//
//   - entries are base names. A flattened listing naturally yields the full
//     key, and a caller that path.Joins the entry onto the directory it asked
//     for then builds "dir/dir/file".
//   - the listing is one level deep. A prefix listing naturally returns the
//     whole subtree; grandchildren must be collapsed into a single directory
//     entry, not enumerated.
//   - entries are sorted, as [ocflfs.DirEntriesFS] documents, so callers may
//     rely on the order without sorting defensively.
//
// Unlike Remove and RemoveAll this entry point takes no knobs: the two
// backends agree on every case here, including a missing directory, which S3
// reports as fs.ErrNotExist rather than as an empty listing.
func TestDirEntries(t *testing.T, fsys ocflfs.WriteFS) {
	t.Helper()
	ctx := context.Background()

	dirFS, ok := fsys.(ocflfs.DirEntriesFS)
	if !ok {
		t.Fatal("backend does not implement ocflfs.DirEntriesFS")
	}

	// A deliberately unsorted creation order, so a backend that happens to
	// return insertion order fails the sorted-order assertion.
	const base = "imptest-direntries"
	for _, name := range []string{
		base + "/zeta.txt",
		base + "/alpha.txt",
		base + "/sub/child.txt",
		base + "/sub/deeper/grandchild.txt",
		base + "/middle.txt",
	} {
		_, err := fsys.Write(ctx, name, strings.NewReader("x"))
		be.NilErr(t, err)
	}

	collect := func(t *testing.T, dir string) ([]fs.DirEntry, error) {
		t.Helper()
		var entries []fs.DirEntry
		for entry, err := range dirFS.DirEntries(ctx, dir) {
			if err != nil {
				return entries, err
			}
			// The iterator yields an entry or an error, never both.
			if entry == nil {
				t.Fatalf("DirEntries(%q) yielded a nil entry with a nil error", dir)
			}
			entries = append(entries, entry)
		}
		return entries, nil
	}

	names := func(entries []fs.DirEntry) []string {
		out := make([]string, 0, len(entries))
		for _, e := range entries {
			out = append(out, e.Name())
		}
		return out
	}

	t.Run("yields base names one level deep, sorted", func(t *testing.T) {
		entries, err := collect(t, base)
		be.NilErr(t, err)

		got := names(entries)
		be.AllEqual(t, []string{"alpha.txt", "middle.txt", "sub", "zeta.txt"}, got)

		// Sorted order is a documented promise, asserted independently of the
		// expected set above so a future change to the fixture cannot quietly
		// drop it.
		be.True(t, slices.IsSorted(got))

		// "sub" is the collapsed grandparent of deeper/grandchild.txt, and it
		// must be reported as a directory; the plain files must not be.
		byName := map[string]fs.DirEntry{}
		for _, e := range entries {
			byName[e.Name()] = e
		}
		be.True(t, byName["sub"].IsDir())
		be.False(t, byName["alpha.txt"].IsDir())
	})

	t.Run("descends one level at a time", func(t *testing.T) {
		entries, err := collect(t, base+"/sub")
		be.NilErr(t, err)
		// "deeper" appears as a directory; "grandchild.txt" belongs to the
		// next level down and must not surface here.
		be.AllEqual(t, []string{"child.txt", "deeper"}, names(entries))
	})

	t.Run("missing directory yields ErrNotExist", func(t *testing.T) {
		// S3 has no directories, so this is the case where the flat backend
		// has to go out of its way to agree: a prefix matching nothing lists
		// empty, and the backend must turn that into fs.ErrNotExist rather
		// than yielding an empty sequence that a caller would read as "the
		// directory exists and is empty".
		_, err := collect(t, base+"/no-such-dir")
		be.True(t, err != nil)
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})

	t.Run("invalid path is rejected", func(t *testing.T) {
		for _, dir := range []string{"../escape", "/absolute", "a/../b", ""} {
			_, err := collect(t, dir)
			if err == nil {
				t.Errorf("DirEntries(%q) yielded no error, want a rejection", dir)
				continue
			}
			if !errors.Is(err, fs.ErrInvalid) {
				t.Errorf("DirEntries(%q) error = %v, want one matching fs.ErrInvalid", dir, err)
			}
		}
	})
}
