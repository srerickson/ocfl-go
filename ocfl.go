// This module is an implementation of the Oxford Common File Layout (OCFL)
// specification. The top-level package provides version-independent
// functionality. The ocflv1 package provides the bulk of implementation.
package ocfl

import "github.com/srerickson/ocfl/digest"

const (
	// package version
	Version       = "0.0.16"
	ExtensionsDir = "extensions"
)

var (
	Spec1_0 = Spec{1, 0}
	Spec1_1 = Spec{1, 1}
)

// AlgRegistry returns the global digest algorithm registry
func AlgRegistry() *digest.Registry {
	return digest.DefaultRegistry()
}
