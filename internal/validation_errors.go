package internal

import (
	"errors"
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

// checks if the err is a *ValidationErr. If it isn't
// it creates one using err and code.
func asValidationErr(err error, code *OCFLCodeErr) *ValidationErr {
	var vErr *ValidationErr
	if errors.As(err, &vErr) {
		return vErr
	}
	return &ValidationErr{
		err:  err,
		code: code,
	}
}

// ContentDiffErr represents an error due to
// unexpected content changes
type ContentDiffErr struct {
	Added       []string
	Removed     []string
	Modified    []string
	RenamedFrom []string
	RenamedTo   []string
}

func (e *ContentDiffErr) Error() string {
	return "unexpected files changes"
}
