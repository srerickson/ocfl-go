package ocfl

import (
	"log/slog"

	"github.com/hashicorp/go-multierror"
)

type Validation struct {
	SkipDigests bool
	Logger      *slog.Logger
}

type ValidationResult struct {
	Fatal   *multierror.Error
	Warning *multierror.Error
}

func (r *ValidationResult) Err() error {
	if r.Fatal == nil {
		return nil
	}
	return r.Fatal.ErrorOrNil()
}
