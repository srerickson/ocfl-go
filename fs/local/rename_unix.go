//go:build !windows

package local

import "os"

// renameReplace replaces the entry at dst with the entry at src, moving
// src onto dst (src no longer exists afterwards). On POSIX, os.Rename
// performs the replacement atomically and can replace any non-directory
// entry at dst, including an existing symlink — the link entry itself is
// replaced, never its referent. If src is a symlink, the link entry is
// moved: dst becomes a symlink to the same referent, which is never
// followed. On Windows, replacement cannot be done with os.Rename and
// goes through renameReplaceWindows instead (see rename_windows.go).
func renameReplace(src, dst string) error {
	return os.Rename(src, dst)
}
