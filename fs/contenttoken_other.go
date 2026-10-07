//go:build !(linux || openbsd || dragonfly || solaris || darwin || freebsd || netbsd || windows)

package fs

// sysContentToken returns "": content tokens from stat structures aren't
// supported on this platform.
func sysContentToken(any) string { return "" }
