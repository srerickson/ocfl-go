package ocflv1

import (
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/logging"
	"github.com/srerickson/ocfl/validation"
	"golang.org/x/exp/slog"
)

type validationOptions struct {
	// Validation errors/warnings logged here. Defaults to logr.Discard()
	Logger *slog.Logger

	// MaxErrs sets the capacity (max size)s for the returned validation.Result.
	// Defaults to 100. Use -1 for no limit.
	MaxErrs int

	// Skip object validation when validating a storage root
	SkipObjects bool

	// Digests will not be validated
	SkipDigests bool // don't validate object digiests

	// if the OCFL version cannot be determined, validation errors will
	// reference this version of the OCFL ocfl.
	FallbackOCFL ocfl.Spec

	// Algorithm Registry
	AlgRegistry *digest.Registry

	// Validation should add errors to an existing result set.
	result *validation.Result
}

func defaultValidationOptions() *validationOptions {
	return &validationOptions{
		AlgRegistry:  ocfl.AlgRegistry(),
		Logger:       logging.DefaultLogger(),
		FallbackOCFL: ocflv1_0,
		MaxErrs:      100,
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

// ValidationAlgRegistry sets the registry of available algorithms
// that can be used in an inventory fixity
func ValidationAlgRegistry(r *digest.Registry) ValidationOption {
	return func(opts *validationOptions) {
		opts.AlgRegistry = r
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
