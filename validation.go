package ocfl

import (
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"time"
)

// ValidateObject validates the object at root
func ValidateObject(root fs.FS) error {
	obj, err := NewObjectReader(root)
	if err != nil {
		return wrapValidationErrors(err)
	}
	return obj.Validate()
}

func (obj *ObjectReader) Validate() error {
	return obj.Inventory.CUEValudate()
}

func wrapValidationErrors(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return &ErrE034
	}
	if _, ok := err.(*json.UnmarshalTypeError); ok {
		if strings.Contains(err.Error(), `Inventory.head`) {
			return &ErrE034
		}
	}
	if _, ok := err.(*time.ParseError); ok {
		return &ErrE049
	}
	return err

}
