package ocfl

import (
	"io/fs"

	"github.com/srerickson/ocfl/internal"
	"github.com/srerickson/ocfl/validation"
)

const Version = "0.0.0"

type ObjectReader internal.ObjectReader

func (obj *ObjectReader) LogicalFS() (fs.FS, error) {
	return (*internal.ObjectReader)(obj).LogicalFS()
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
func ValidateObject(fsys fs.FS) *validation.Result {
	return internal.ValidateObject(fsys)
}
