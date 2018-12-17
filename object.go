package ocfl

import (
	"github.com/srerickson/ocfl/namaste"
)

const (
	namasteObjectTValue = `ocfl_object_1.0`
	namasteObjectFValue = "ocfl_object_\n"
)

// Object represents an OCFL Object
type Object struct {
	Path      string
	inventory Inventory
}

// Validate returns any validation errors for the OCFL Object at o.Path
func (o *Object) Validate() (bool, error) {
	match, err := namaste.MatchTypePattern(o.Path, namasteObjectTValue)
	if err != nil {
		return false, err
	}
	if !match {
		return false, nil
	}
	//ETC ...
	return true, nil
}
