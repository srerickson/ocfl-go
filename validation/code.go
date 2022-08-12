package validation

import "github.com/srerickson/ocfl"

// Code represents an OCFL Validation code referencing a specification.
// see https://ocfl.io/validation/validation-codes.html
type Code struct {
	// Num represents the validation code itself (e.g., "E001"). The code can be
	// shared by multiple versions of the ocfl.
	Num string
	// a map of references to specs by spec number
	refs map[ocfl.Spec]*Ref
}

// Ref is a code description and reference to a spec
type Ref struct {
	Code        string
	Description string
	URL         string
}

func NewCode(num string, refs map[ocfl.Spec]*Ref) Code {
	return Code{
		Num:  num,
		refs: refs,
	}
}

func (c Code) Ref(v ocfl.Spec) *Ref {
	r, exists := c.refs[v]
	if !exists {
		return nil
	}
	return &Ref{
		Code:        c.Num,
		Description: r.Description,
		URL:         r.URL,
	}
}
