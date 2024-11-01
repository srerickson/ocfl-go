package digest

import (
	"errors"
	"fmt"
)

var (
	// ErrUnknown: a digest algorithm was not recognize
	ErrUnknown = errors.New("unrecognized digest algorithm")
	// ErrMissing: missing an expected digest algorithm
	ErrMissing = errors.New("missing an expected digest algorithm")

	// built-in Alg register
	defaultRegister = NewAlgorithmRegistry(SHA512, SHA256, SHA1, MD5, BLAKE2B)
)

// AlgorithmRegistry is an immutable collection of Algorithm indexed by ID.
type AlgorithmRegistry struct {
	algs map[string]Algorithm
}

// NewAlgorithmRegistry returns a Registry for the given extension algs
func NewAlgorithmRegistry(algs ...Algorithm) AlgorithmRegistry {
	newR := AlgorithmRegistry{
		algs: make(map[string]Algorithm, len(algs)),
	}
	for _, alg := range algs {
		newR.algs[alg.ID()] = alg
	}
	return newR
}

// Get returns the Alg for the given id or ErrUnknown if the algorithm is not
// present in the registry.
func (r AlgorithmRegistry) Get(id string) (Algorithm, error) {
	alg, ok := r.algs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	return alg, nil
}

// GetAny returns a slice of Algorithms for any of the algorithm ids in the registry.
// The returned slace may be nil.
func (r AlgorithmRegistry) GetAny(ids ...string) []Algorithm {
	var algs []Algorithm
	for _, id := range ids {
		alg, err := r.Get(id)
		if err != nil {
			continue
		}
		algs = append(algs, alg)
	}
	return algs
}

// GetAny returns a slice of Algorithms for any of the algorithm ids in ids
// found in the registry.
func (r AlgorithmRegistry) All() []Algorithm {
	algs := make([]Algorithm, 0, len(r.algs))
	for _, alg := range r.algs {
		algs = append(algs, alg)
	}
	return algs
}

// MustGet is like Get except it panics if the registry does not include
// the an algorithm with the given id.
func (r AlgorithmRegistry) MustGet(id string) Algorithm {
	alg, err := r.Get(id)
	if err != nil {
		panic(err)
	}
	return alg
}

// NewDigester returns a digester for the given id, which must an Alg registered
// in r.
func (r AlgorithmRegistry) NewDigester(id string) (Digester, error) {
	alg, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	return alg.Digester(), nil
}

// NewMultiDigester returns a MultiDigester using algs from the register.
func (r AlgorithmRegistry) NewMultiDigester(algs ...string) *MultiDigester {
	return NewMultiDigester(r.GetAny(algs...)...)
}

// Append returns a new Registry that includes algs from r plus additional algs.
// If the added algs have the same id as those in r, the new registry will use
// new algs.
func (r AlgorithmRegistry) Append(algs ...Algorithm) AlgorithmRegistry {
	newR := AlgorithmRegistry{
		algs: make(map[string]Algorithm, len(r.algs)+len(algs)),
	}
	for _, alg := range r.algs {
		newR.algs[alg.ID()] = alg
	}
	for _, alg := range algs {
		newR.algs[alg.ID()] = alg
	}
	return newR
}

// IDs returns IDs of all Algs in r.
func (r AlgorithmRegistry) IDs() []string {
	names := make([]string, 0, len(r.algs))
	for name := range r.algs {
		names = append(names, name)
	}
	return names
}

// Len returns number of algs in the registry
func (r AlgorithmRegistry) Len() int {
	return len(r.algs)
}

// DefaultRegistry returns a Register built-in digest algorithms: sha512, sha256,
// sha1, md5, and blake2b.
func DefaultRegistry() AlgorithmRegistry { return defaultRegister }
