package ocfl

import (
	"log/slog"

	"github.com/hashicorp/go-multierror"
)

type SpecErrCode struct {
	Spec        Spec   // OCFL spec that the code refers to
	Code        string // Validation error code from OCFL Spec
	Description string // error description from spec
	URL         string // URL to the OCFL specification for the error
}

// Validation Error is an error that includes a reference
// to a validation error code from an OCFL spec.
type ValidationError struct {
	Err error
	Ref *SpecErrCode
}

func (verr *ValidationError) Error() string {
	return verr.Err.Error()
}

func (ver *ValidationError) Unwrap() error {
	return ver.Err
}

type ValidateOptions struct {
	Spec        Spec
	SkipDigests bool
	Logger      *slog.Logger
}

type ValidationOption func(*ValidateOptions)

func ValidationSkipDigest() ValidationOption {
	return func(opts *ValidateOptions) {
		opts.SkipDigests = true
	}
}

func ValidationLogger(logger *slog.Logger) ValidationOption {
	return func(opts *ValidateOptions) {
		opts.Logger = logger
	}
}

type Validation struct {
	fatal *multierror.Error
	warn  *multierror.Error
}

func (r *Validation) AddFatal(errs ...error) {
	r.fatal = multierror.Append(r.fatal, errs...)
}

func (r *Validation) AddWarn(errs ...error) {
	r.warn = multierror.Append(r.warn, errs...)
}

func (r *Validation) Err() error {
	if r.fatal == nil {
		return nil
	}
	return r.fatal.ErrorOrNil()
}

func (r *Validation) WarnErr() error {
	if r.warn == nil {
		return nil
	}
	return r.warn.ErrorOrNil()
}

func (r *Validation) Errors() []error {
	if r.fatal == nil {
		return nil
	}
	return r.fatal.Errors
}

func (r *Validation) WarnErrors() []error {
	if r.warn == nil {
		return nil
	}
	return r.warn.Errors
}

func (r *Validation) Add(r2 *Validation) {
	r.AddFatal(r2.fatal.Errors...)
	r.AddWarn(r2.warn.Errors...)
}
