package internal

import (
	"errors"
	"fmt"
)

var _ ValidationErr = (*validationErr)(nil)

// ValidationErr is an error returned from validation check
type ValidationErr interface {
	error
	Code() string
	Description() string
	URI() string
}

// ValidationErr is an error returned from validation check
type validationErr struct {
	code *OCFLCodeErr // code from spec
	err  error        // internal error
}

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

func (verr *validationErr) Unwrap() error {
	return verr.err
}

func (verr *validationErr) Error() string {
	code := "??"
	const format = "[%s] %s"
	if verr.code != nil {
		code = verr.code.Code
	}
	return fmt.Sprintf(format, code, verr.err.Error())
}

func (verr *validationErr) Code() string {
	if verr.code == nil {
		return ""
	}
	return verr.code.Code
}

func (verr *validationErr) Description() string {
	if verr.code == nil {
		return ""
	}
	return verr.code.Description
}

func (verr *validationErr) URI() string {
	if verr.code == nil {
		return ""
	}
	return verr.code.Description
}

// checks if the err is a *ValidationErr. If it isn't
// it creates one using err and code.
func asValidationErr(err error, code *OCFLCodeErr) *validationErr {
	var vErr *validationErr
	if errors.As(err, &vErr) {
		return vErr
	}
	return &validationErr{
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
