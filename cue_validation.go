package ocfl

import (
	"cuelang.org/go/cuego"
	"github.com/srerickson/ocfl/cue"
)

func init() {
	cuego.MustConstrain(&Inventory{}, cue.OCFL)
}

//CUEVAlidate validates using cue
func (i *Inventory) CUEValudate() error {
	return cuego.Validate(i)
}
