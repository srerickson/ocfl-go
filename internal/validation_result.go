package internal

import (
	"errors"
	"fmt"
	"io/fs"
)

var _ ValidationResult = (*validationResult)(nil)

type ValidationResult interface {
	error
	Fatal() []ValidationErr
	Warning() []ValidationErr
	Valid() bool
}

// validationResult is an error returned from validation check
type validationResult struct {
	fatal    []ValidationErr
	warnings []ValidationErr
}

// ValidateObject validates the object at root
func ValidateObject(root fs.FS) ValidationResult {
	vr := &validationResult{}
	obj, err := NewObjectReader(root)
	if err != nil {
		return vr.AddFatal(err, nil)
	}
	vr.Merge(obj.Validate())
	return vr
}

func (r *validationResult) Error() string {
	return fmt.Sprintf("encountered %d fatal error(s) and %d warning(s)", len(r.fatal), len(r.warnings))
}

func (r *validationResult) Fatal() []ValidationErr {
	return r.fatal
}

func (r *validationResult) Warning() []ValidationErr {
	return r.warnings
}

func (r *validationResult) Valid() bool {
	return len(r.fatal) == 0
}

func (r *validationResult) Merge(err error) bool {
	// TODO - How to handle nil r, err?
	var r2 *validationResult
	if errors.As(err, &r2) {
		if r2 == nil {
			return false
		}
		r.fatal = append(r.fatal, r2.fatal...)
		r.warnings = append(r.warnings, r2.warnings...)
		return true
	}
	return false
}

func (r *validationResult) AddFatal(err error, code *OCFLCodeErr) *validationResult {
	// merge ignores code!
	if !r.Merge(err) {
		r.fatal = append(r.fatal, asValidationErr(err, code))
	}
	return r
}

func (r *validationResult) AddWarn(err error, code *OCFLCodeErr) *validationResult {
	if !r.Merge(err) {
		r.warnings = append(r.warnings, asValidationErr(err, code))
	}
	return r
}
