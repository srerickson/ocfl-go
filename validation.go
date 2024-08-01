package ocfl

import (
	"log/slog"

	"github.com/hashicorp/go-multierror"
)

type ValidateOptions struct {
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

type ValidationResult struct {
	fatal *multierror.Error
	warn  *multierror.Error
}

func (r *ValidationResult) AddFatal(errs ...error) {
	r.fatal = multierror.Append(r.fatal, errs...)
}

func (r *ValidationResult) AddWarn(errs ...error) {
	r.warn = multierror.Append(r.warn, errs...)
}

func (r *ValidationResult) Err() error {
	if r.fatal == nil {
		return nil
	}
	return r.fatal.ErrorOrNil()
}

func (r *ValidationResult) WarnErr() error {
	if r.warn == nil {
		return nil
	}
	return r.warn.ErrorOrNil()
}

func (r *ValidationResult) Errors() []error {
	if r.fatal == nil {
		return nil
	}
	return r.fatal.Errors
}

func (r *ValidationResult) WarnErrors() []error {
	if r.warn == nil {
		return nil
	}
	return r.warn.Errors
}
