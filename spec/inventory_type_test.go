package spec_test

import (
	"testing"

	"github.com/matryer/is"
	"github.com/srerickson/ocfl/spec"
)

type invTypeTest struct {
	in    string
	out   spec.Num
	valid bool
}

var invTypeTable = []invTypeTest{
	// valid
	{"https://ocfl.io/1.0/spec/#inventory", spec.Num{1, 0}, true},
	{"https://ocfl.io/1.1/spec/#inventory", spec.Num{1, 1}, true},
	// invalid
	{"https://ocfl.io/1/spec/#inventory", spec.Num{0, 0}, false},
	{"https://ocfl.io/spec/#inventory", spec.Num{0, 0}, false},
}

func TestParseInventoryType(t *testing.T) {
	is := is.New(t)
	for _, t := range invTypeTable {
		inv := spec.InventoryType{}
		err := inv.UnmarshalText([]byte(t.in))
		if t.valid {
			is.NoErr(err)
		} else {
			is.True(err != nil)
		}
		is.Equal(inv.Num, t.out)
	}
}
