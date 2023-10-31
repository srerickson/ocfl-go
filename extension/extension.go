package extension

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"hash"

	"golang.org/x/crypto/blake2b"
)

const (
	// extension name key for config.json
	extensionName = "extensionName"
	// extensions directory name
	extensions = "extensions"
)

// global register of extensions
var register = map[string]func() Extension{
	ext0002: Ext0002,
	ext0003: Ext0003,
	ext0004: Ext0004,
	ext0006: Ext0006,
	ext0007: Ext0007,
}

var (
	ErrMarshal         = errors.New("extension config doesn't include '" + extensionName + "' string")
	ErrNotLayout       = errors.New("not a layout extension")
	ErrUnknown         = errors.New("unrecognized extension")
	ErrInvalidLayoutID = errors.New("invalid object id for layout")
)

type Extension interface {
	Name() string // Name returns the extension name
}

// Get returns a new instance of the named extension with default values.
func Get(name string) (Extension, error) {
	extfunc, ok := register[name]
	if !ok {
		return nil, fmt.Errorf("%w: '%s'", ErrUnknown, name)
	}
	return extfunc(), nil
}

// Register adds the extension returned by the extfunc to the extension
// register. The extension instance returned by extfunc must have default
// values.
func Register(extfunc func() Extension) {
	ext := extfunc()
	register[ext.Name()] = extfunc
}

// Registered returns slice of strings with all registered extension names
func Registered() []string {
	names := make([]string, 0, len(register))
	for name := range register {
		names = append(names, name)
	}
	return names
}

// IsRegistered returns true if the named extension is present in the register
func IsRegistered(name string) bool {
	_, ok := register[name]
	return ok
}

// Unmarshal decodes the extension config json and returns a new extension instance.
func Unmarshal(jsonBytes []byte) (Extension, error) {
	type tmpConfig struct {
		Name string `json:"extensionName"`
	}
	var tmp tmpConfig
	if err := json.Unmarshal(jsonBytes, &tmp); err != nil {
		return nil, err
	}
	config, err := Get(tmp.Name)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(jsonBytes, config); err != nil {
		return nil, err
	}
	return config, nil
}

// Layout is extension that provides a function for resolving object IDs to
// Storage Root Paths
type Layout interface {
	Extension
	Resolve(id string) (path string, err error)
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
