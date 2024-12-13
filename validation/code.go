package validation

// ValidationCode represents a validation error code defined in an
// OCFL specification. See https://ocfl.io/1.1/spec/validation-codes.html
type ValidationCode struct {
	Spec        string // OCFL spec version that the code refers to (e.g '1.1')
	Code        string // Validation error code from OCFL Spec
	Description string // error description from spec
	URL         string // URL to the OCFL specification for the error
}
