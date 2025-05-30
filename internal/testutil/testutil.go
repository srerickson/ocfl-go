package testutil

import (
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/fs/local"
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

// make a temporary local.FS for writing with contents
// of srcdir copied into it
func TmpLocalFS(t *testing.T, srcDir string) *local.FS {
	t.Helper()
	tmpDir := t.TempDir()
	if err := os.CopyFS(tmpDir, os.DirFS(srcDir)); err != nil {
		t.Fatal(err)
	}
	fsys, err := local.NewFS(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	return fsys
}
