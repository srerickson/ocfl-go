package local

// tempname_test.go pins the UTF-8 safety contract of tempFileName: the temp
// name it returns must always fit NAME_MAX (255 bytes), must stay valid
// UTF-8 even when the target's base name is a long multibyte string that a
// raw byte slice would split mid-rune, and must keep the "<truncated base>"
// prefix correlation with the target so a temp file can be matched back to
// the file it belongs to.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/carlmjohnson/be"
)

// dotLen and suffixLen are the fixed parts of a tempFileName result that
// wrap the (possibly truncated) base: the leading dot, the ".tmp-"
// separator, and the 16 hex random digits.
const (
	dotLen    = len(".")          // leading dot
	suffixLen = len(".tmp-") + 16 // ".tmp-" separator plus hex random
)

// basePortion extracts the truncated base embedded in a tempFileName result.
func basePortion(temp string) string {
	return temp[dotLen : len(temp)-suffixLen]
}

func TestTempFileNameUTF8(t *testing.T) {
	t.Run("short name is unchanged", func(t *testing.T) {
		temp := tempFileName("plain.bin")
		be.True(t, strings.HasPrefix(temp, ".plain.bin.tmp-"))
		be.Equal(t, dotLen+len("plain.bin")+suffixLen, len(temp))
		be.True(t, len(temp) <= 255)
		be.True(t, utf8.ValidString(temp))
	})

	t.Run("multibyte name under limit is not truncated", func(t *testing.T) {
		base := strings.Repeat("é", 100) // 200 bytes, under the 233-byte budget
		temp := tempFileName(base)
		be.True(t, strings.HasPrefix(temp, "."+base+".tmp-"))
		be.Equal(t, dotLen+len(base)+suffixLen, len(temp))
		be.True(t, utf8.ValidString(temp))
		createTempFileOnDisk(t, temp)
	})

	t.Run("long ASCII name truncates exactly at budget", func(t *testing.T) {
		base := strings.Repeat("a", 300)
		temp := tempFileName(base)
		trunc := basePortion(temp)
		be.Equal(t, 233, len(trunc))
		be.True(t, strings.HasPrefix(base, trunc))
		be.Equal(t, 255, len(temp))
		be.True(t, utf8.ValidString(temp))
		createTempFileOnDisk(t, temp)
	})

	t.Run("long multibyte name truncates on a rune boundary", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			rune  string
			count int
		}{
			{"latin-1 accent e", "é", 128}, // 256 bytes, 2 bytes/rune
			{"emoji", "😀", 64},             // 256 bytes, 4 bytes/rune
		} {
			t.Run(tc.name, func(t *testing.T) {
				base := strings.Repeat(tc.rune, tc.count)
				t.Logf("base byte length: %d (>255 required)", len(base))
				be.True(t, len(base) > 255)
				temp := tempFileName(base)

				// The full temp name must fit NAME_MAX and stay valid UTF-8:
				// no mid-rune truncation anywhere in the name.
				be.True(t, len(temp) <= 255)
				be.True(t, utf8.ValidString(temp))

				// Prefix correlation: the truncated base embedded in the temp
				// name is a prefix of the original base, and the truncation
				// only ever shortened it (never to zero for these sizes).
				trunc := basePortion(temp)
				be.True(t, strings.HasPrefix(base, trunc))
				be.True(t, len(trunc) > 0)
				be.True(t, len(trunc) <= 233)

				// The temp name must actually be creatable on the filesystem.
				createTempFileOnDisk(t, temp)
			})
		}
	})
}

// createTempFileOnDisk verifies that name can be created with O_EXCL in a
// fresh directory on the local filesystem, which is exactly what
// createTempFile does with tempFileName's result and what strict filesystems
// reject when a name is invalid UTF-8 or exceeds NAME_MAX.
func createTempFileOnDisk(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, tempPerm)
	be.NilErr(t, err)
	be.NilErr(t, f.Close())
}
