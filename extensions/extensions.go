package extensions

import (
	"errors"
	"fmt"
)

// global register of extensions
var register = map[string]func() Extension{
	Ext0002: func() Extension { return NewLayoutFlatDirect() },
	Ext0003: func() Extension { return NewLayoutHashIDTuple() },
	Ext0004: func() Extension { return NewLayoutHashTuple() },
	Ext0006: func() Extension { return NewLayoutFlatOmitPrefix() },
	Ext0007: func() Extension { return NewLayoutTupleOmitPrefix() },
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
		return nil, fmt.Errorf("%w: '%s'", ErrUnknown, name)
	}
	return ext(), nil
}
