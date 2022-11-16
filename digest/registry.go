package digest

import (
	"errors"
	"fmt"
)

var ErrUnknownAlg = errors.New("unknown digest algorithm")

// Registry stores available digest algorithms by their id.
type Registry struct {
	m map[string]Alg
}

var defaultRegistry *Registry

// DefaultRegistry returns the gobal algorithm registry
func DefaultRegistry() *Registry {
	if defaultRegistry == nil {
		defaultRegistry = NewRegistry()
	}
	return defaultRegistry
}

// Get is short for DefaultRegistry().Get(id)
func Get(id string) (Alg, error) {
	return DefaultRegistry().Get(id)
}

// NewRegistry returns a new registry with built-in algorithms
func NewRegistry() *Registry {
	reg := &Registry{}
	reg.Add(builtin...)
	return reg
}

func (r *Registry) Add(algs ...Alg) {
	if r.m == nil {
		r.m = map[string]Alg{}
	}
	for _, alg := range algs {
		r.m[alg.ID()] = alg
	}
}

func (r Registry) Get(id string) (Alg, error) {
	alg, ok := r.m[id]
	if !ok {
		return nil, fmt.Errorf("%w: '%s'", ErrUnknownAlg, id)
	}
	return alg, nil
}
