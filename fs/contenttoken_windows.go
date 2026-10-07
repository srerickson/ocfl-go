package fs

import (
	"strconv"
	"syscall"
)

// sysContentToken returns a content token from the file attributes sys, or ""
// if sys isn't one. Windows doesn't provide a change time, so the token only
// has the last write time.
func sysContentToken(sys any) string {
	attrs, ok := sys.(*syscall.Win32FileAttributeData)
	if !ok || attrs == nil {
		return ""
	}
	return "stat:" + strconv.FormatInt(attrs.LastWriteTime.Nanoseconds(), 10)
}
