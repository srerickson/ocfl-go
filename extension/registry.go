package extension

import (
	"encoding/json"
	"fmt"
)

// Registry is an immutable container of Extension constructors.
type Registry struct {
	exts map[string]func() Extension
}

// New returns a new Extension value for the given extension name or returns an
// error if the extension is not present in the registry. The returned Extension
// should have default values (or zero-values where defaults are not defined).
func (r Registry) New(name string) (Extension, error) {
	extfunc, ok := r.exts[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknown, name)
	}
	return extfunc(), nil
}

// NewLayout is the same as New with an additional check that the extension is a
// layout.
func (r Registry) NewLayout(name string) (Layout, error) {
	ext, err := r.New(name)
	if err != nil {
		return nil, err
	}
	if layout, isLayout := ext.(Layout); isLayout {
		return layout, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrNotLayout, name)
}

// Append returns a new Registry that includes extension constructors from r
// plus additional constructors. If the added extension constructors have the same
// name as those in r, the new registry will use the added constructor.
func (r Registry) Append(extFns ...func() Extension) Registry {
	newR := Registry{
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
func (r Registry) Names() []string {
	names := make([]string, 0, len(r.exts))
	for name := range r.exts {
		names = append(names, name)
	}
	return names
}

// Unmarshal decodes the extension config json and returns a new extension instance.
func (r Registry) Unmarshal(jsonBytes []byte) (Extension, error) {
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

// NewRegistry returns a Registry for the given extension constructors
func NewRegistry(extFns ...func() Extension) Registry {
	newR := Registry{
		exts: make(map[string]func() Extension, len(extFns)),
	}
	for _, fn := range extFns {
		ext := fn()
		newR.exts[ext.Name()] = fn
	}
	return newR
}

// DefaultRegistry returns a new Registry with default Extension
// constructors.
func DefaultRegistry() Registry {
	return NewRegistry(baseExtensions...)
}

// Get returns a new instance of the named extension with default values.
// DEPRECATED.
func Get(name string) (Extension, error) {
	return DefaultRegistry().New(name)
}
