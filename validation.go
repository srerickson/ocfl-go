package ocfl

import (
	"log/slog"

	"github.com/hashicorp/go-multierror"
)

// Validation is used to configure and track results from a validation process.
type Validation struct {
	logger *slog.Logger
	// skipDigests bool
	fatal *multierror.Error
	warn  *multierror.Error
	files map[string]*validationFileInfo
}

func (v *Validation) AddInventoryFiles(inv Inventory) error {
	if v.files == nil {
		v.files = map[string]*validationFileInfo{}
	}
	primaryAlg := inv.DigestAlgorithm()
	var err error
	inv.Manifest().EachPath(func(name string, primaryDigest string) bool {
		allDigests := inv.GetFixity(primaryDigest)
		allDigests[primaryAlg] = primaryDigest
		current := v.files[name]
		if current == nil {
			v.files[name] = &validationFileInfo{
				expected: allDigests,
			}
			return true
		}
		for alg, newDigest := range allDigests {
			currDigest := current.expected[alg]
			if currDigest == "" {
				current.expected[alg] = newDigest
				continue
			}
			if currDigest == newDigest {
				continue
			}
			// digest conflict
			err = &DigestErr{
				Name:     name,
				Alg:      alg,
				Got:      newDigest,
				Expected: currDigest,
			}
			return false
		}
		return true
	})
	return err
}

// AddErrors adds v2's fatal and warning errors from to v.
func (v *Validation) AddErrors(v2 *Validation) {
	v.fatal = multierror.Append(v.fatal, v2.fatal.Errors...)
	v.warn = multierror.Append(v.warn, v2.warn.Errors...)
	// v.skipDigests = v.skipDigests && v2.skipDigests
}

// AddFatal adds fatal errors to the validation
func (v *Validation) AddFatal(errs ...error) {
	v.fatal = multierror.Append(v.fatal, errs...)
}

// AddWarn adds warning errors to the validation
func (v *Validation) AddWarn(errs ...error) {
	v.warn = multierror.Append(v.warn, errs...)
}

// Err returns an error wrapping all the validation's fatal errors, or nil if
// there are no fatal errors.
func (v *Validation) Err() error {
	if v.fatal == nil {
		return nil
	}
	return v.fatal.ErrorOrNil()
}

// Errors returns a slice of all the fatal errors.q
func (v *Validation) Errors() []error {
	if v.fatal == nil {
		return nil
	}
	return v.fatal.Errors
}

// Logger returns the validation's logger, which is nil by default.
func (v *Validation) Logger() *slog.Logger {
	return v.logger
}

// Options returns a ValidationOptions that replicates the options
// used to configure v.
func (v *Validation) Options() ValidationOption {
	return func(v2 *Validation) {
		v2.logger = v.logger
		// v2.skipDigests = v.skipDigests
	}
}

// SkipDigests returns true if the validation is configured to skip digest
// checks. It is false by default.
// func (v *Validation) SkipDigests() bool {
// 	return v.skipDigests
// }

// WarnErr returns an error wrapping all the validation's warning errors, or nil
// if there are none.
func (v *Validation) WarnErr() error {
	if v.warn == nil {
		return nil
	}
	return v.warn.ErrorOrNil()
}

// WarnErrors returns a slice of all the warning errors.
func (v *Validation) WarnErrors() []error {
	if v.warn == nil {
		return nil
	}
	return v.warn.Errors
}

// NewValidation constructs a new *Validation with the given
// options
func NewValidation(opts ...ValidationOption) *Validation {
	v := &Validation{}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

type ValidationOption func(*Validation)

// func ValidationSkipDigest() ValidationOption {
// 	return func(opts *Validation) {
// 		opts.skipDigests = true
// 	}
// }

func ValidationLogger(logger *slog.Logger) ValidationOption {
	return func(v *Validation) {
		v.logger = logger
	}
}

// ValidationCode represents a validation error code defined in an
// OCFL specificaiton. See https://ocfl.io/1.1/spec/validation-codes.html
type ValidationCode struct {
	Spec        Spec   // OCFL spec that the code refers to
	Code        string // Validation error code from OCFL Spec
	Description string // error description from spec
	URL         string // URL to the OCFL specification for the error
}

// ValidationError is an error that includes a reference
// to a validation error code from the OCFL spec.
type ValidationError struct {
	Err  error
	Code *ValidationCode
}

func (ver *ValidationError) Error() string {
	return ver.Err.Error()
}

func (ver *ValidationError) Unwrap() error {
	return ver.Err
}

type validationFileInfo struct {
	expected DigestSet
	exists   bool
}
