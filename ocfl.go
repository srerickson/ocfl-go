// This module is an implementation of the Oxford Common File Layout (OCFL)
// specification. The top-level package provides version-independent
// functionality. The ocflv1 package provides the bulk of implementation.
package ocfl

import (
	"runtime"
	"sync/atomic"
)

const (
	// package version
	Version       = "0.0.23"
	ExtensionsDir = "extensions"
)

var (
	digestConcurrency atomic.Int32
	commitConcurrency atomic.Int32
)

// DigestConcurrency is a global configuration for the number  of files to
// digest concurrently.
func DigestConcurrency() int {
	i := digestConcurrency.Load()
	if i < 1 {
		return runtime.NumCPU()
	}
	return int(i)
}

// SetDigestConcurrency sets the max number of files to digest concurrently.
func SetDigestConcurrency(i int) {
	digestConcurrency.Store(int32(i))
}

// XferConcurrency is a global configuration for the maximum number of files
// transferred concurrently during a commit operation. It defaults to
// runtime.NumCPU().
func XferConcurrency() int {
	i := commitConcurrency.Load()
	if i < 1 {
		return runtime.NumCPU()
	}
	return int(i)
}

// SetXferConcurrency sets the maximum number of files transferred concurrently
// during a commit operation.
func SetXferConcurrency(i int) {
	commitConcurrency.Store(int32(i))
}

// User is a generic user information struct
type User struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}
