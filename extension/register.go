package extension

import (
	"encoding/json"
	"fmt"
)

// Register is an immutable container of Extension constructors.
type Register struct {
	exts map[string]func() Extension
}

// New returns a new Extension value for the given extension name or returns an
// error if the extension is not present in the register. The returned Extension
// should have default values (or zero-values where defaults are not defined).
func (r Register) New(name string) (Extension, error) {
	extfunc, ok := r.exts[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknown, name)
	}
	return extfunc(), nil
}

// NewLayout is the same as New with an additional check that the extension is a
// layout.
func (r Register) NewLayout(name string) (Layout, error) {
	ext, err := r.New(name)
	if err != nil {
		return nil, err
	}
	if layout, isLayout := ext.(Layout); isLayout {
		return layout, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrNotLayout, name)
}

// Append returns a new Register that includes extension constructors from r
// plus additional constructors. If the added extension constructors have the same
// name as those in r, the new register will use the added constructor.
func (r Register) Append(extFns ...func() Extension) Register {
	newR := Register{
		exts: make(map[string]func() Extension, len(r.exts)+len(extFns)),
	}
	for n, fn := range r.exts {
		newR.exts[n] = fn
	}
	for _, fn := range extFns {
		newR.exts[fn().Name()] = fn
	}
	return newR
}

// Names returns names of all Extensions constructors in r.
func (r Register) Names() []string {
	names := make([]string, 0, len(r.exts))
	for name := range r.exts {
		names = append(names, name)
	}
	return names
}

// Unmarshal decodes the extension config json and returns a new extension instance.
func (r Register) Unmarshal(jsonBytes []byte) (Extension, error) {
	type tmpConfig struct {
		Name string `json:"extensionName"`
	}
	var tmp tmpConfig
	if err := json.Unmarshal(jsonBytes, &tmp); err != nil {
		return nil, err
	}
	config, err := r.New(tmp.Name)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(jsonBytes, config); err != nil {
		return nil, err
	}
	return config, nil
}

// NewRegister returns a Register for the given extension constructors
func NewRegister(extFns ...func() Extension) Register {
	newR := Register{
		exts: make(map[string]func() Extension, len(extFns)),
	}
	for _, fn := range extFns {
		ext := fn()
		newR.exts[ext.Name()] = fn
	}
	return newR
}

// DefaultRegister returns a new Register with default Extension
// constructors.
func DefaultRegister() Register {
	return NewRegister(baseExtensions...)
}

// Get returns a new instance of the named extension with default values.
// DEPRECATED.
func Get(name string) (Extension, error) {
	return DefaultRegister().New(name)
}
