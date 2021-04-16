package ocfl

import (
	"io/fs"

	"github.com/srerickson/ocfl/internal"
)

const Version = "0.0.0"

type ObjectReader internal.ObjectReader
type ValidationResult internal.ValidationResult

func (obj *ObjectReader) Open(name string) (fs.File, error) {
	return (*internal.ObjectReader)(obj).Open(name)
}

// NewObjectReader returns an ObjectReader with root at fsys.
func NewObjectReader(fsys fs.FS) (*ObjectReader, error) {
	obj, err := internal.NewObjectReader(fsys)
	if err != nil {
		return nil, err
	}
	return (*ObjectReader)(obj), nil
}

// ValidateObject returns ValidationResults for object at fsys.
func ValidateObject(fsys fs.FS) ValidationResult {
	return internal.ValidateObject(fsys)
}
