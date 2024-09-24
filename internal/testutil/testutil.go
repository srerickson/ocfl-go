package testutil

import (
	"errors"
	"slices"
	"testing"

	"github.com/srerickson/ocfl-go"
)

// ErrorsIncludeOCFLCode checks that errs includes at least one
// ocfl.ValidationError with the given code value.
func ErrorsIncludeOCFLCode(t *testing.T, ocflCode string, errs ...error) {
	t.Helper()
	var foundCodes []string
	for _, err := range errs {
		var vCode *ocfl.ValidationError
		if errors.As(err, &vCode) {
			foundCodes = append(foundCodes, vCode.Code)
		}
	}
	if !slices.Contains(foundCodes, ocflCode) {
		t.Errorf("OCFL validation code %q not in found validation codes %v", ocflCode, foundCodes)
	}
}
