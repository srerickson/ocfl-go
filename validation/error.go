package validation

import (
	"errors"
	"fmt"
)

// VErr is an error returned from validation check
type VErr struct {
	code *OCFLCodeErr // code from spec
	err  error        // internal error
}

func (verr *VErr) Unwrap() error {
	return verr.err
}

func (verr *VErr) Error() string {
	code := "??"
	const format = "[%s] %s"
	if verr.code != nil {
		code = verr.code.Code
	}
	return fmt.Sprintf(format, code, verr.err.Error())
}

func (verr *VErr) Code() string {
	if verr.code == nil {
		return ""
	}
	return verr.code.Code
}

func (verr *VErr) Description() string {
	if verr.code == nil {
		return ""
	}
	return verr.code.Description
}

func (verr *VErr) URI() string {
	if verr.code == nil {
		return ""
	}
	return verr.code.Description
}

// AsVErr checks if the err is a *ValidationErr. If it isn't
// it creates one using err and code.
func AsVErr(err error, code *OCFLCodeErr) *VErr {
	var vErr *VErr
	if errors.As(err, &vErr) {
		return vErr
	}
	return &VErr{
		err:  err,
		code: code,
	}
}
