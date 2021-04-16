package ocfl

import (
	"io/fs"

	"github.com/srerickson/ocfl/internal"
)

const version = "0.0.0"

// NewObjectReader returns an ObjectReader with root at fsys.
func NewObjectReader(fsys fs.FS) (*internal.ObjectReader, error) {
	return internal.NewObjectReader(fsys)
}

// ValidateObject returns ValidationResults for object at fsys.
func ValidateObject(fsys fs.FS) *internal.ValidationResult {
	return internal.ValidateObject(fsys)
}
