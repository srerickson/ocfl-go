package validation

import (
	"errors"
	"fmt"
)

// Result is a validation result. Contains zero or more fatal errors and warning errors.
type Result struct {
	fatal    []*VErr
	warnings []*VErr
}

func (r *Result) Error() string {
	return fmt.Sprintf("encountered %d fatal error(s) and %d warning(s)", len(r.fatal), len(r.warnings))
}

func (r *Result) Fatal() []*VErr {
	return r.fatal
}

func (r *Result) Warning() []*VErr {
	return r.warnings
}

func (r *Result) Valid() bool {
	return len(r.fatal) == 0
}

func (r *Result) Merge(err error) bool {
	// TODO - How to handle nil r, err?
	var r2 *Result
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

func (r *Result) AddFatal(err error, code *OCFLCodeErr) *Result {
	// merge ignores code!
	if !r.Merge(err) {
		r.fatal = append(r.fatal, AsVErr(err, code))
	}
	return r
}

func (r *Result) AddWarn(err error, code *OCFLCodeErr) *Result {
	if !r.Merge(err) {
		r.warnings = append(r.warnings, AsVErr(err, code))
	}
	return r
}
