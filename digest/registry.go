package digest

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var ErrUnknownAlg = errors.New("unknown digest algorithm")

// RegistryCtxKey is used to access the digest Registry from a context.Context
type RegistryCtxKey struct{}

var builtin = map[string]Alg{
	SHA512id:  SHA512(),
	SHA256id:  SHA256(),
	SHA224id:  SHA224(),
	SHA1id:    SHA1(),
	MD5id:     MD5(),
	BLAKE2Bid: BLAKE2B(),
}

// digest registry
type Registry struct {
	algs sync.Map
}

// NewRegistry returns a new registry with built-in algorithms
func NewRegistry() *Registry {
	reg := &Registry{}
	reg.Add([]Alg{
		SHA512(),
		SHA256(),
		SHA224(),
		SHA1(),
		MD5(),
		BLAKE2B(),
	}...)
	return reg
}

func (r *Registry) Add(algs ...Alg) {
	for _, alg := range algs {
		r.algs.Store(alg.ID(), alg)
	}
}

func (r Registry) Get(id string) (Alg, error) {
	alg, ok := r.algs.Load(id)
	if !ok {
		return nil, fmt.Errorf("%w: '%s'", ErrUnknownAlg, id)
	}
	return alg.(Alg), nil
}

func RegistryFromContext(ctx context.Context) *Registry {
	v := ctx.Value(RegistryCtxKey{})
	if v == nil {
		return NewRegistry()
	}
	return v.(*Registry)
}

func ContextWithRegistry(ctx context.Context, r *Registry) context.Context {
	return context.WithValue(ctx, RegistryCtxKey{}, r)
}

// Get is used to retrieve a built-in Alg
func Get(id string) (Alg, error) {
	alg, ok := builtin[id]
	if !ok {
		return nil, fmt.Errorf("%w: '%s'", ErrUnknownAlg, id)
	}
	return alg, nil
}
