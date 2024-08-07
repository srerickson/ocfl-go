package ocfl

import (
	"errors"
	"log/slog"

	"github.com/hashicorp/go-multierror"
)

type Validation struct {
	logger      *slog.Logger
	skipDigests bool
	fatal       *multierror.Error
	warn        *multierror.Error
}

// AddFatal adds fatal errors to the validation
func (v *Validation) AddFatal(errs ...error) {
	v.fatal = multierror.Append(v.fatal, errs...)
	if v.logger == nil {
		return
	}
	for _, err := range errs {
		var validErr *ValidationError
		switch {
		case errors.As(err, &validErr):
			v.logger.Error(err.Error(), "ocfl_code", validErr.ValidationCode.Code)
		default:
			v.logger.Error(err.Error())
		}
	}
}

// AddWarn adds warning errors to the validation
func (v *Validation) AddWarn(errs ...error) {
	v.warn = multierror.Append(v.warn, errs...)
	if v.logger == nil {
		return
	}
	for _, err := range errs {
		var validErr *ValidationError
		switch {
		case errors.As(err, &validErr):
			v.logger.Warn(err.Error(), "ocfl_code", validErr.ValidationCode.Code)
		default:
			v.logger.Warn(err.Error())
		}
	}
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

// SkipDigests returns true if the validation is configured to skip digest
// checks. It is false by default.
func (v *Validation) SkipDigests() bool {
	return v.skipDigests
}

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

type ValidationOption func(*Validation)

func ValidationSkipDigest() ValidationOption {
	return func(opts *Validation) {
		opts.skipDigests = true
	}
}

func ValidationLogger(logger *slog.Logger) ValidationOption {
	return func(v *Validation) {
		v.logger = logger
	}
}

// NewObjectValidation constructs a new *Validation with the given
// options
func NewObjectValidation(opts ...ValidationOption) *ObjectValidation {
	v := &ObjectValidation{}
	for _, opt := range opts {
		opt(&v.Validation)
	}
	return v
}

// ObjectValidation is used to configure and track results from a validation process.
type ObjectValidation struct {
	Validation
	// not sure if this belongs here.
	files map[string]*validationFileInfo
}

// AddExistingContent sets the existence status for a content file in the
// validation state.
func (v *ObjectValidation) AddExistingContent(name string) {
	if v.files == nil {
		v.files = map[string]*validationFileInfo{}
	}
	if v.files[name] == nil {
		v.files[name] = &validationFileInfo{}
	}
	v.files[name].exists = true
}

func (v *ObjectValidation) AddInventoryDigests(inv ReadInventory) error {
	if v.files == nil {
		v.files = map[string]*validationFileInfo{}
	}
	primaryAlg := inv.DigestAlgorithm()
	allErrors := &multierror.Error{}
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
		if err := current.expected.Add(allDigests); err != nil {
			var digestError *DigestError
			if errors.As(err, &digestError) {
				digestError.Name = name
			}
			allErrors = multierror.Append(allErrors, err)
		}
		return true
	})
	return allErrors.ErrorOrNil()
}

func (v *ObjectValidation) MissingContent() func(func(name string) bool) {
	return func(yield func(string) bool) {
		for name, entry := range v.files {
			if !entry.exists && len(entry.expected) > 0 {
				if !yield(name) {
					return
				}
			}
		}
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
	ValidationCode
	Err error
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
