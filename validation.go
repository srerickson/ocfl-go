package ocfl

import "github.com/hashicorp/go-multierror"

type Validation struct {
	SkipDigests bool
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
