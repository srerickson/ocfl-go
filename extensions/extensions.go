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

func Get(name string) (Extension, error) {
	ext, ok := register[name]
	if !ok {
		return nil, fmt.Errorf("%w: '%s'", ErrUnknown, name)
	}
	return ext(), nil
}
