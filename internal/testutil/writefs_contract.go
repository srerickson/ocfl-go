package testutil

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

// WriteFSRemoveContract configures the shared WriteFS.Remove contract test
// (TestWriteFSRemoveContract) with the backend-specific part of the contract.
//
// WriteFS.Remove documents (fs/fs.go) that removing a missing file returns an
// error satisfying errors.Is(err, fs.ErrNotExist) on every backend, and that
// removing the top-level directory (".") is always an error that must not
// affect the storage root. Name "." is the only name whose error may be
// backend-specific: the S3 backend reports fs.ErrNotExist, while the local
// backend returns a descriptive *fs.PathError instead.
type WriteFSRemoveContract struct {
	// RemoveDotIsNotExist reports whether the backend's Remove(".") error
	// satisfies errors.Is(err, fs.ErrNotExist), per the WriteFS.Remove docs:
	// true for the S3 backend, false for the local backend.
	RemoveDotIsNotExist bool
}

// TestWriteFSRemoveContract asserts the WriteFS.Remove contract behavior that
// all backends share, against whichever backend fsys implements. Call it from
// a backend's own test files with the backend's WriteFSRemoveContract.
//
// The test covers:
//   - removing a missing file returns an error satisfying
//     errors.Is(err, fs.ErrNotExist) (the Option B missing-file contract),
//     reported as a *fs.PathError with Op "remove";
//   - removing "." returns a non-nil *fs.PathError naming ".", never touches
//     the storage root (a file written before the call must still be present
//     and readable afterwards), and satisfies errors.Is(err, fs.ErrNotExist)
//     exactly when contract.RemoveDotIsNotExist says the backend documents
//     that behavior.
func TestWriteFSRemoveContract(t *testing.T, fsys ocflfs.WriteFS, contract WriteFSRemoveContract) {
	t.Helper()
	ctx := context.Background()

	t.Run("remove missing file returns ErrNotExist", func(t *testing.T) {
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
			probe   = "root-probe.txt"
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
		// errors.Is must behave exactly as the WriteFS.Remove doc promises for
		// "." — the one name whose error the contract permits to be
		// backend-dependent.
		be.Equal(t, contract.RemoveDotIsNotExist, errors.Is(err, fs.ErrNotExist))
		// The storage root must be unaffected: the probe file written before
		// Remove(".") must still be present and readable.
		file, err := fsys.OpenFile(ctx, probe)
		be.NilErr(t, err)
		data, err := io.ReadAll(file)
		be.NilErr(t, err)
		be.NilErr(t, file.Close())
		be.Equal(t, payload, string(data))
	})
}
