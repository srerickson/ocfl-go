package ocfl_test

import (
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
)

func TestOCLFV1_ValidateInventory(t *testing.T) {
	inv := &ocfl.RawInventory{}
	v := ocfl.OCFLv1_0.ValidateInventory(inv)
	be.Nonzero(t, v.Err())
}
