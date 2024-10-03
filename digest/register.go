package digest

import (
	"errors"
	"fmt"
)

var (
	ErrUnknown = errors.New("unrecognized digest algorithm")
	Defaults   = DefaultRegister()
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

// New returns the Alg for the given id or ErrUnknown if the algorithm is not
// present in the register.
func (r Register) New(id string) (Alg, error) {
	alg, ok := r.algs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	return alg, nil
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

// Names returns names of all Alg IDs in r.
func (r Register) Names() []string {
	names := make([]string, 0, len(r.algs))
	for name := range r.algs {
		names = append(names, name)
	}
	return names
}

// DefaultRegister returns a new Register with default Alg constructors.
func DefaultRegister() Register { return NewRegister(builtin...) }
