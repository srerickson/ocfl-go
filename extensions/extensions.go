package extensions

import (
	"errors"
	"fmt"
)

// global register of extensions
var register = map[string]func() Extension{
	Ext0002: NewLayoutFlatDirect,
	Ext0003: NewLayoutHashIDTuple,
	Ext0004: NewLayoutHashTuple,
	Ext0006: NewLayoutFlatOmitPrefix,
	Ext0007: NewLayoutTupleOmitPrefix,
}

var ErrNotLayout = errors.New("not a layout extension")
var ErrUnknown = errors.New("unrecognized extension")

type Extension interface {
	Name() string
}

// Layout is the interface for layout extensions
type Layout interface {
	Extension
	NewFunc() (LayoutFunc, error)
}

// LayoutFunc is a function that maps an object id to a path in the storage root
// or returns an error if the id is invalid
type LayoutFunc func(string) (string, error)

func Get(name string) (Extension, error) {
	ext, ok := register[name]
	if !ok {
		return nil, fmt.Errorf("%s: %w", name, ErrUnknown)
	}
	return ext(), nil
}
