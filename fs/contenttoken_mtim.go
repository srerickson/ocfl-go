//go:build linux || openbsd || dragonfly || solaris

package fs

import (
	"strconv"
	"syscall"
)

// sysContentToken returns a content token from the stat structure sys, or ""
// if sys isn't one.
func sysContentToken(sys any) string {
	stat, ok := sys.(*syscall.Stat_t)
	if !ok || stat == nil {
		return ""
	}
	return "stat:" + strconv.FormatInt(stat.Mtim.Nano(), 10) + ":" + strconv.FormatInt(stat.Ctim.Nano(), 10)
}
