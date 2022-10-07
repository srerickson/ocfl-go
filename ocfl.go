// This repository provides the github.com/srerickson/ocfl module, an
// implementation of the OCFL specification. The top-level package provides
// version-independent types, functions, and variables. The ocflv1 package
// provides the bulk of implementation. It implements both OCFL v1.0 and v1.1.
// Command line tools can be found in `cmd`.
package ocfl

const (
	// package version
	Version = "0.0.9"

	// log levels
	LevelDebug   = 10
	LevelWarning = 5
)
