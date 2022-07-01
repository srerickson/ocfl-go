package spec_test

import (
	"testing"

	"github.com/matryer/is"
	spec "github.com/srerickson/ocfl/spec"
)

type vNumTest struct {
	in    string
	out   spec.Num
	valid bool
}

var vNumTable = []vNumTest{
	// valid
	{`1.0`, spec.Num{1, 0}, true},
	{`1.1`, spec.Num{1, 1}, true},
	{`100.11`, spec.Num{100, 11}, true},
	{`100.119999`, spec.Num{100, 119999}, true},

	// invalid
	{`1.00`, spec.Num{0, 0}, false},
	{`10`, spec.Num{0, 0}, false},
	{`0`, spec.Num{0, 0}, false},
	{``, spec.Num{0, 0}, false},
	{`v1`, spec.Num{0, 0}, false},
	{`1.0001`, spec.Num{0, 0}, false},
	{`01.1`, spec.Num{0, 0}, false},
	{`1.00`, spec.Num{0, 0}, false},
	{`0.0`, spec.Num{0, 0}, false},
}

func TestVersionParse(t *testing.T) {
	is := is.New(t)
	for _, t := range vNumTable {
		v := spec.Num{}
		err := spec.Parse(t.in, &v)
		if t.valid {
			is.NoErr(err)
		} else {
			is.True(err != nil)
		}
		is.Equal(v, t.out)
	}
}

func TestCmp(t *testing.T) {
	v1 := spec.MustParse("2.2")
	table := map[spec.Num]int{
		spec.MustParse("2.0"): 1,
		spec.MustParse("1.0"): 1,
		spec.MustParse("2.2"): 0,
		spec.MustParse("2.3"): -1,
		spec.MustParse("3.2"): -1,
	}

	for v2, exp := range table {
		got := v1.Cmp(v2)
		if got != exp {
			t.Errorf("comparing %s and %s: got %d, expected %d", v1, v2, got, exp)
		}
	}

}
