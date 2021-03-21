package ocfl

import (
	"io/fs"
)

// ValidateObject validates the object at root
func ValidateObject(root fs.FS) error {
	obj, err := NewObjectReader(root)
	if err != nil {
		return err
	}
	return obj.Validate()
}
