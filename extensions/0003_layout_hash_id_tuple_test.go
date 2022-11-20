package extensions_test

import (
	"testing"

	"github.com/srerickson/ocfl/extensions"
)

func TestLayoutHashIDTuple(t *testing.T) {
	table := map[string]string{
		`object-01`:        `3c0/ff4/240/object-01`,
		`..hor/rib:le-$id`: `487/326/d8c/%2e%2ehor%2frib%3ale-%24id`,
		`..Hor/rib:lè-$id`: `373/529/21a/%2e%2eHor%2frib%3al%c3%a8-%24id`,
	}
	l := extensions.NewLayoutHashIDTuple()
	for in, exp := range table {
		testLayoutExt(t, l, in, exp)
	}
	// with 4-tuple
	l.TupleNum = 4
	table = map[string]string{
		"ark:123/abc": "a47/817/83d/cec/ark%3a123%2fabc",
	}
	for in, exp := range table {
		testLayoutExt(t, l, in, exp)
	}
}
