package ocfl

import (
	"fmt"
)

// OCFLCodeErr represents an OCFL Validation Codes:
// see https://ocfl.io/validation/validation-codes.html
type OCFLCodeErr struct {
	Description string // description from spec
	Code        string // code from spec
	URI         string // reference URI from spec
}

// Error implements the Error interface for ObjectValidationErr
func (err *OCFLCodeErr) Error() string {
	return fmt.Sprintf(`[%s] %s`, err.Code, err.Description)
}

// ValidationErr is an error returned from validation check
type ValidationErr struct {
	code *OCFLCodeErr // code from spec
	err  error        // internal error
}

func (verr *ValidationErr) Unwrap() error {
	return verr.err
}

func (verr *ValidationErr) Error() string {
	code := "??"
	const format = "[%s] %s"
	if verr.code != nil {
		code = verr.code.Code
	}
	return fmt.Sprintf(format, code, verr.err.Error())
}

func (verr *ValidationErr) Code() *OCFLCodeErr {
	return verr.code
}
