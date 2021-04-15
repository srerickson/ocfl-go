package ocfl

import (
	"errors"
	"fmt"
	"io/fs"
)

// ValidationResult is an error returned from validation check
type ValidationResult struct {
	Fatal    []*ValidationErr
	Warnings []error
}

func (r *ValidationResult) Error() string {
	return fmt.Sprintf("encountered %d fatal error(s) and %d warning(s)", len(r.Fatal), len(r.Warnings))
}

func (r *ValidationResult) Merge(err error) bool {
	var r2 *ValidationResult
	if errors.As(err, &r2) {
		r.Fatal = append(r.Fatal, r2.Fatal...)
		r.Warnings = append(r.Warnings, r2.Warnings...)
		return true
	}
	return false
}

func (r *ValidationResult) AddFatal(err error, code *OCFLCodeErr) *ValidationResult {
	// merge ignores code!
	if !r.Merge(err) {
		r.Fatal = append(r.Fatal, asValidationErr(err, code))
	}
	return r
}

func (r *ValidationResult) AddWarn(err error, code *OCFLCodeErr) *ValidationResult {
	if !r.Merge(err) {
		r.Warnings = append(r.Warnings, asValidationErr(err, code))
	}
	return r
}

func (r *ValidationResult) Valid() bool {
	return len(r.Fatal) == 0 && len(r.Warnings) == 0
}

// ValidateObject validates the object at root
func ValidateObject(root fs.FS) *ValidationResult {
	vr := &ValidationResult{}
	obj, err := NewObjectReader(root)
	if err != nil {
		return vr.AddFatal(err, nil)
	}
	return obj.Validate()
}
