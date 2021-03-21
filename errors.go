package ocfl

import (
	"errors"
	"fmt"
)

// ErrInvalidObj is a generic OCFL validation error and
// parent of all ObjectValidationErr instances
var ErrInvalidObj error = errors.New(`invalid OCFL`)

// ObjectValidationErr represents an OCFL Object Validation Error:
// see https://ocfl.io/validation/validation-codes.html
type ObjectValidationErr struct {
	Name        string // short name (not from spec)
	Description string // description from spec
	Code        string // code from spec
	URI         string // reference URI from spec
}

// Error implements the Error interface for ObjectValidationErr
func (err *ObjectValidationErr) Error() string {
	if err.Name == "" {
		return fmt.Sprintf(`%s [%s]`, err.Description, err.Code)
	}
	return fmt.Sprintf(`%s [%s]`, err.Description, err.Code)
}

// Unwrap implements the Unwrap interface for ObjectValidationErr
func (err *ObjectValidationErr) Unwrap() error {
	return ErrInvalidObj
}
