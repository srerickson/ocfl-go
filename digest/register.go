package digest

import (
	"errors"
	"fmt"
	"io"
)

var (
	// ErrUnknown: a digest algorithm was not recognize
	ErrUnknown = errors.New("unrecognized digest algorithm")
	// ErrMissing: missing an expected digest algorithm
	ErrMissing = errors.New("missing an expected digest algorithm")

	// built-in Alg register
	defaultRegister = NewRegistry(SHA512, SHA256, SHA1, MD5, BLAKE2B)
)

// Registry is an immutable collection of available Algs.
type Registry struct {
	algs map[string]Algorithm
}

// NewRegistry returns a Registry for the given extension algs
func NewRegistry(algs ...Algorithm) Registry {
	newR := Registry{
		algs: make(map[string]Algorithm, len(algs)),
	}
	for _, alg := range algs {
		newR.algs[alg.ID()] = alg
	}
	return newR
}

// Get returns the Alg for the given id or ErrUnknown if the algorithm is not
// present in the registry.
func (r Registry) Get(id string) (Algorithm, error) {
	alg, ok := r.algs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	return alg, nil
}

// NewDigester returns a digester for the given id, which must an Alg registered
// in r.
func (r Registry) NewDigester(id string) (Digester, error) {
	alg, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	return alg.Digester(), nil
}

// NewMultiDigester returns a MultiDigester using algs from the register. An
// error is returned if any alg is not defined in the r or if len(algs) < 1.
func (r Registry) NewMultiDigester(algs ...string) (*MultiDigester, error) {
	if len(algs) < 1 {
		return nil, ErrMissing
	}
	writers := make([]io.Writer, 0, len(algs))
	digesters := make(map[string]Digester, len(algs))
	for _, algID := range algs {
		r, err := r.NewDigester(algID)
		if err != nil {
			return nil, err
		}
		digesters[algID] = r
		writers = append(writers, r)
	}
	return &MultiDigester{
		Writer:    io.MultiWriter(writers...),
		digesters: digesters,
	}, nil
}

// Append returns a new Registry that includes algs from r plus additional algs.
// If the added algs have the same id as those in r, the new registry will use
// new algs.
func (r Registry) Append(algs ...Algorithm) Registry {
	newR := Registry{
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
func (r Registry) IDs() []string {
	names := make([]string, 0, len(r.algs))
	for name := range r.algs {
		names = append(names, name)
	}
	return names
}

// DefaultRegister returns a Register built-in digest algorithms: sha512, sha256,
// sha1, md5, and blake2b.
func DefaultRegister() Registry { return defaultRegister }
