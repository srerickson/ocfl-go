//go:build windows

// Windows-specific rename helper: os.Rename cannot replace an existing
// destination file on Windows — it fails with ERROR_ACCESS_DENIED or
// ERROR_ALREADY_EXISTS on some Go and filesystem combinations — so file
// replacement goes through MoveFileEx with MOVEFILE_REPLACE_EXISTING.

package local

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// renameReplace replaces the entry at dst with the entry at src, moving
// src onto dst (src no longer exists afterwards). If src is a symlink,
// the link entry itself is moved — dst becomes a symlink to the same
// referent, which is never followed. On Windows this is always delegated
// to renameReplaceWindows rather than tried with os.Rename first:
// os.Rename itself maps to MoveFileEx(MOVEFILE_REPLACE_EXISTING), so a
// failed os.Rename would already have failed the helper's first strategy,
// and running both would add a guaranteed failing syscall to every
// overwrite. The helper's Remove+Rename fallback is not atomic — a
// failure between the Remove and the Rename leaves no entry at dst.
func renameReplace(src, dst string) error {
	return renameReplaceWindows(src, dst)
}

// renameReplaceWindows replaces the file at dst with the file at src.
// Plain os.Rename is insufficient on Windows because it refuses to
// overwrite an existing destination, so replacement is first attempted with
// MoveFileEx and MOVEFILE_REPLACE_EXISTING, which swaps src over dst
// atomically. If MoveFileEx fails, the helper falls back to the classic
// non-atomic sequence os.Remove(dst) followed by os.Rename(src, dst); a
// failure between those two steps leaves no file at dst, which callers
// should keep in mind. The fallback is skipped when the source is missing,
// so an existing dst is never deleted for nothing. A non-nil error is
// returned only after both strategies have been tried, and it reports the
// MoveFileEx failure as well as any fallback failure so callers can tell
// why the replacement could not be performed.
func renameReplaceWindows(src, dst string) error {
	from, err := windows.UTF16PtrFromString(src)
	if err != nil {
		return fmt.Errorf("windows rename %q -> %q: invalid source path: %w", src, dst, err)
	}
	to, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return fmt.Errorf("windows rename %q -> %q: invalid destination path: %w", src, dst, err)
	}
	moveErr := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING)
	if moveErr == nil {
		return nil
	}
	// If the source is missing, no fallback can succeed: removing dst first
	// would destroy the existing file for nothing.
	if os.IsNotExist(moveErr) {
		return fmt.Errorf("windows rename %q -> %q: MoveFileEx: %w", src, dst, moveErr)
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("windows rename %q -> %q: MoveFileEx failed (%v), remove destination: %w", src, dst, moveErr, err)
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("windows rename %q -> %q: MoveFileEx failed (%v), rename after removing destination: %w", src, dst, moveErr, err)
	}
	return nil
}
