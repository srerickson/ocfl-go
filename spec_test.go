package ocfl_test

import (
	"testing"

	"github.com/matryer/is"
	"github.com/srerickson/ocfl"
)

type vNumTest struct {
	in    string
	out   ocfl.Spec
	valid bool
}

var vNumTable = []vNumTest{
	// valid
	{`1.0`, ocfl.Spec{1, 0}, true},
	{`1.1`, ocfl.Spec{1, 1}, true},
	{`100.11`, ocfl.Spec{100, 11}, true},
	{`100.119999`, ocfl.Spec{100, 119999}, true},

	// invalid
	{`1.00`, ocfl.Spec{0, 0}, false},
	{`10`, ocfl.Spec{0, 0}, false},
	{`0`, ocfl.Spec{0, 0}, false},
	{``, ocfl.Spec{0, 0}, false},
	{`v1`, ocfl.Spec{0, 0}, false},
	{`1.0001`, ocfl.Spec{0, 0}, false},
	{`01.1`, ocfl.Spec{0, 0}, false},
	{`1.00`, ocfl.Spec{0, 0}, false},
	{`0.0`, ocfl.Spec{0, 0}, false},
}

func TestVersionParse(t *testing.T) {
	is := is.New(t)
	for _, t := range vNumTable {
		v := ocfl.Spec{}
		err := ocfl.ParseSpec(t.in, &v)
		if t.valid {
			is.NoErr(err)
		} else {
			is.True(err != nil)
		}
		is.Equal(v, t.out)
	}
}

func TestCmp(t *testing.T) {
	v1 := ocfl.MustParseSpec("2.2")
	table := map[ocfl.Spec]int{
		ocfl.MustParseSpec("2.0"): 1,
		ocfl.MustParseSpec("1.0"): 1,
		ocfl.MustParseSpec("2.2"): 0,
		ocfl.MustParseSpec("2.3"): -1,
		ocfl.MustParseSpec("3.2"): -1,
	}

	for v2, exp := range table {
		got := v1.Cmp(v2)
		if got != exp {
			t.Errorf("comparing %s and %s: got %d, expected %d", v1, v2, got, exp)
		}
	}
}

type invTypeTest struct {
	in    string
	out   ocfl.Spec
	valid bool
}

var invTypeTable = []invTypeTest{
	// valid
	{"https://ocfl.io/1.0/spec/#inventory", ocfl.Spec{1, 0}, true},
	{"https://ocfl.io/1.1/spec/#inventory", ocfl.Spec{1, 1}, true},
	// invalid
	{"https://ocfl.io/1/spec/#inventory", ocfl.Spec{0, 0}, false},
	{"https://ocfl.io/spec/#inventory", ocfl.Spec{0, 0}, false},
}

func TestParseInventoryType(t *testing.T) {
	is := is.New(t)
	for _, t := range invTypeTable {
		inv := ocfl.InvType{}
		err := inv.UnmarshalText([]byte(t.in))
		if t.valid {
			is.NoErr(err)
		} else {
			is.True(err != nil)
		}
		is.Equal(inv.Spec, t.out)
	}
}