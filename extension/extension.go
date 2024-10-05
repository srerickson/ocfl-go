package extension

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"hash"

	"github.com/srerickson/ocfl-go/digest"
	"golang.org/x/crypto/blake2b"
)

const (
	extensionName = "extensionName" // extension name key for config.json
	extensions    = "extensions"    // extensions directory name
)

var (
	ErrMarshal         = errors.New("extension config doesn't include '" + extensionName + "' string")
	ErrNotLayout       = errors.New("not a layout extension")
	ErrUnknown         = errors.New("unrecognized extension name")
	ErrInvalidLayoutID = errors.New("invalid object id for layout")

	// built-in extensions
	baseExtensions = []func() Extension{
		Ext0002,
		Ext0003,
		Ext0004,
		Ext0006,
		Ext0007,
	}
)

// Extension is implemented by types that represent specific OCFL Extensions.
// See https://github.com/OCFL/extensions
type Extension interface {
	Name() string // Name returns the extension name
}

// Layout is an extension that provides a function for resolving object IDs to
// Storage Root Paths
type Layout interface {
	Extension
	// Resolve resolves an object ID into a storage root path
	Resolve(id string) (path string, err error)
	// Valid returns an error if the layout configuation is invalid
	Valid() error
}

// DigestAlgorithm is an extension that provides a collection of availble
// digest algorithms
type DigestAlgorithm interface {
	Extension
	Algorithms() digest.Register
}

func getAlg(name string) hash.Hash {
	switch name {
	case `sha512`:
		return sha512.New()
	case `sha256`:
		return sha256.New()
	case `sha1`:
		return sha1.New()
	case `md5`:
		return md5.New()
	case `blake2b-512`:
		h, err := blake2b.New512(nil)
		if err != nil {
			panic("creating new blake2b hash")
		}
		return h
	default:
		return nil
	}
}
