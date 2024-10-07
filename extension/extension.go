package extension

import (
	"errors"
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

// Base is a type that can be embedded by types that implement
// the Extension.
type Base struct {
	ExtensionName string `json:"extensionName"`
}

func (b Base) Name() string { return string(b.ExtensionName) }

// Layout is an extension that provides a function for resolving object IDs to
// Storage Root Paths
type Layout interface {
	Extension
	// Resolve resolves an object ID into a storage root path
	Resolve(id string) (path string, err error)
	// Valid returns an error if the layout configuation is invalid
	Valid() error
}
