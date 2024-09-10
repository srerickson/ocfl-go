package ocfl

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/hashicorp/go-multierror"
)

// Validation represents multiple fatal errors and warning errors.
type Validation struct {
	fatal *multierror.Error
	warn  *multierror.Error
}

// Add adds all fatal errors and warnings from another validation to v.
func (v *Validation) Add(v2 *Validation) {
	if v2 == nil {
		return
	}
	v.AddFatal(v2.Errors()...)
	v.AddWarn(v2.WarnErrors()...)
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

// ObjectValidation is used to configure and track results from an object validation process.
type ObjectValidation struct {
	Validation

	globals     Config
	logger      *slog.Logger
	skipDigests bool
	files       map[string]*validationFileInfo
}

// NewObjectValidation constructs a new *Validation with the given
// options
func NewObjectValidation(opts ...ObjectValidationOption) *ObjectValidation {
	v := &ObjectValidation{}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// Add adds and logs all fatal errors and warning from the validation
func (v *ObjectValidation) Add(v2 *Validation) {
	if v2 == nil {
		return
	}
	v.AddFatal(v2.Errors()...)
	v.AddWarn(v2.WarnErrors()...)
}

// PrefixAdd adds and logs all fatal errors and warning from the valiation,
// prepending each error with the prefix.
func (v *ObjectValidation) PrefixAdd(prefix string, v2 *Validation) {
	if v2 == nil {
		return
	}
	for _, err := range v2.Errors() {
		v.AddFatal(fmt.Errorf("%s: %w", prefix, err))
	}
	for _, err := range v2.WarnErrors() {
		v.AddWarn(fmt.Errorf("%s: %w", prefix, err))
	}
}

// AddFatal adds fatal errors to the validation
func (v *ObjectValidation) AddFatal(errs ...error) {
	v.Validation.AddFatal(errs...)
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

// AddWarn adds warning errors to the object validation and logs the errors
// using the object validations logger, if set.
func (v *ObjectValidation) AddWarn(errs ...error) {
	v.Validation.AddWarn(errs...)
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

// AddInventoryDigests adds digests from the inventory's manifest and fixity
// entries to the object validation for later verification. An error is returned
// if any name/digests entries in the inventory conflic with an existing
// name/digest entry already added to the object validation. The returned error
// wraps a slice of *DigestError values.
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

// Logger returns the validation's logger, which is nil by default.
func (v *ObjectValidation) Logger() *slog.Logger {
	return v.logger
}

// MissingContent returns an iterator the yields the names of files that appear
// in an inventory added to the validation but were not marked as existing.
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

// SkipDigests returns true if the validation is configured to skip digest
// checks. It is false by default.
func (v *ObjectValidation) SkipDigests() bool {
	return v.skipDigests
}

// UnexpectedContent returns an iterator that yields the names of existing files
// that were not included in an inventory manifest.
func (v *ObjectValidation) UnexpectedContent() func(func(name string) bool) {
	return func(yield func(string) bool) {
		for name, entry := range v.files {
			if entry.exists && len(entry.expected) == 0 {
				if !yield(name) {
					return
				}
			}
		}
	}
}

// ExistingContent digests returns an iterator that yields the names and digests
// of files that exist and were reference in the inventory added to the
// valiation.
func (v *ObjectValidation) ExistingContentDigests() func(func(name string, digests DigestSet) bool) {
	return func(yield func(string, DigestSet) bool) {
		for name, entry := range v.files {
			if entry.exists && len(entry.expected) > 0 {
				if !yield(name, entry.expected) {
					return
				}
			}
		}
	}
}

type ObjectValidationOption func(*ObjectValidation)

func ValidationSkipDigest() ObjectValidationOption {
	return func(opts *ObjectValidation) {
		opts.skipDigests = true
	}
}

func ValidationLogger(logger *slog.Logger) ObjectValidationOption {
	return func(v *ObjectValidation) {
		v.logger = logger
	}
}

type validationFileInfo struct {
	expected DigestSet
	exists   bool
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
