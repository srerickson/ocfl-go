// This module is an implementation of the Oxford Common File Layout (OCFL)
// specification. The top-level package provides version-independent
// functionality. The ocflv1 package provides the bulk of implementation.
package ocfl

import (
	"runtime"
)

const (
	// package version
	Version       = "0.0.17"
	ExtensionsDir = "extensions"
)

var (
	Spec1_0 = Spec{1, 0}
	Spec1_1 = Spec{1, 1}

	digestConcurrency = -1
)

func SetDigestConcurrency(i int) {
	digestConcurrency = i
}

func DigestConcurrency() int {
	if digestConcurrency < 1 {
		return runtime.NumCPU()
	}
	return digestConcurrency
}

// // AlgRegistry returns the global digest algorithm registry
// func AlgRegistry() *digest.Registry {
// 	return digest.DefaultRegistry()
// }
