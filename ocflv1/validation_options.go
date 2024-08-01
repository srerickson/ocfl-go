package ocflv1

import (
	"log/slog"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/logging"
	"github.com/srerickson/ocfl-go/validation"
)

type validationOptions struct {
	// Validation errors/warnings logged here. Loggin is disabled by default
	Logger *slog.Logger

	// MaxErrs sets the capacity (max size)s for the returned validation.Result.
	// Defaults to 100. Use -1 for no limit.
	MaxErrs int

	// Skip object validation when validating a storage root
	SkipObjects bool

	// Skip digests validation when validating objects
	SkipDigests bool

	// if the OCFL version cannot be determined, validation errors will
	// reference this version of the OCFL spec.
	FallbackOCFL ocfl.Spec

	// Validation should add errors to an existing result set.
	result *validation.Result
}

func defaultValidationOptions() *validationOptions {
	return &validationOptions{
		FallbackOCFL: ocfl.Spec1_0,
		MaxErrs:      100,
		Logger:       logging.DisabledLogger(),
	}
}

// ValidationOption is a type used to configure the behavior
// of several Validation functions in the package.
type ValidationOption func(*validationOptions)

// ValidationLogger sets the logger where validation errors are logged.
func ValidationLogger(l *slog.Logger) ValidationOption {
	return func(opts *validationOptions) {
		opts.Logger = l
	}
}

// ValidationMaxErrs sets the limit on the number of fatal error and warning
// errors (each) in the returned validation.Result. The default is 100. Use -1
// to set no limit.
func ValidationMaxErrs(max int) ValidationOption {
	return func(opts *validationOptions) {
		opts.MaxErrs = max
	}
}

// SkipObjects is used to skip object validation when validating storage roots.
func SkipObjects() ValidationOption {
	return func(opts *validationOptions) {
		opts.SkipObjects = true
	}
}

// SkipDigest is used to skip file digest validation when validating an Object.
func SkipDigests() ValidationOption {
	return func(opts *validationOptions) {
		opts.SkipDigests = true
	}
}

// FallbackOCFL is used to set the OCFL spec referenced in error messages when
// an OCFL spec version cannot be determined during validation. Default is OCFL
// v1.0.
func FallbackOCFL(spec ocfl.Spec) ValidationOption {
	return func(opts *validationOptions) {
		opts.FallbackOCFL = spec
	}
}

func copyValidationOptions(newOpts *validationOptions) ValidationOption {
	return func(opts *validationOptions) {
		*opts = *newOpts
	}
}

func appendResult(r *validation.Result) ValidationOption {
	return func(opts *validationOptions) {
		opts.result = r
	}
}

func validationSetup(vops []ValidationOption) (*validationOptions, *validation.Result) {
	opts := defaultValidationOptions()
	for _, o := range vops {
		o(opts)
	}
	result := opts.result
	if result == nil {
		result = validation.NewResult(opts.MaxErrs)
	}
	return opts, result
}
