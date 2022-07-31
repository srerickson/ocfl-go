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
	l, ok := extensions.NewLayoutHashIDTuple().(*extensions.LayoutHashIDTuple)
	if !ok {
		t.Fatal("not a layout")
	}
	f, err := l.NewFunc()
	if err != nil {
		t.Fatal(err)
	}
	for in, exp := range table {
		out, err := f(in)
		if err != nil {
			t.Error(f)
		}
		if out != exp {
			t.Errorf("got %s, expected %s", out, exp)
		}
	}

	// with 4-tuple
	l.TupleNum = 4
	table = map[string]string{
		"ark:123/abc": "a47/817/83d/cec/ark%3a123%2fabc",
	}
	f, err = l.NewFunc()
	if err != nil {
		t.Fatal(err)
	}
	for in, exp := range table {
		out, err := f(in)
		if err != nil {
			t.Error(f)
		}
		if out != exp {
			t.Errorf("got %s, expected %s", out, exp)
		}
	}

}
