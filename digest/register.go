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
	ErrMissing = errors.New("missing a required digest algorithm")

	// built-in Alg register
	builtinRegister = NewRegister(SHA512, SHA256, SHA1, MD5, BLAKE2B)
)

// Register is an immutable container of Algs.
type Register struct {
	algs map[string]Alg
}

// NewRegister returns a Register for the given extension algs
func NewRegister(algs ...Alg) Register {
	newR := Register{
		algs: make(map[string]Alg, len(algs)),
	}
	for _, alg := range algs {
		newR.algs[alg.ID()] = alg
	}
	return newR
}

// Get returns the Alg for the given id or ErrUnknown if the algorithm is not
// present in the register.
func (r Register) Get(id string) (Alg, error) {
	alg, ok := r.algs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	return alg, nil
}

// NewDigester returns a digester for the given id, which must an Alg registered
// in r.
func (r Register) NewDigester(id string) (Digester, error) {
	alg, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	return alg.Digester(), nil
}

// NewMultiRegsiter returns a MultiDigester using algs from the register. An
// error is returned if any alg is not defined in the r or if len(algs) < 1.
func (r Register) NewMultiDigester(algs ...string) (*MultiDigester, error) {
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

// Append returns a new Register that includes algs from r plus additional algs.
// If the added algs have the same id as those in r, the new register will use
// new algs.
func (r Register) Append(algs ...Alg) Register {
	newR := Register{
		algs: make(map[string]Alg, len(r.algs)+len(algs)),
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
func (r Register) IDs() []string {
	names := make([]string, 0, len(r.algs))
	for name := range r.algs {
		names = append(names, name)
	}
	return names
}

// DefaultRegister returns a new Register with built-in Algs (sha512, sha256,
// sha1, md5, and blake2b).
func DefaultRegister() Register { return builtinRegister }
