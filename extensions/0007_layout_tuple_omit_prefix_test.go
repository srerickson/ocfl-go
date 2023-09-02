package extensions_test

import (
	"testing"

	"github.com/srerickson/ocfl-go/extensions"
)

func TestLayoutTupleOmitPrefix(t *testing.T) {
	l := extensions.NewLayoutTupleOmitPrefix()
	l.TupleSize = 4
	l.TupleNum = 2
	l.Padding = "left"
	l.Reverse = true
	tests := map[string]string{
		"namespace:12887296":                            "6927/8821/12887296",
		"urn:uuid:6e8bc430-9c3a-11d9-9669-0800200c9a66": "66a9/c002/6e8bc430-9c3a-11d9-9669-0800200c9a66",
		"abc123": "321c/ba00/abc123",
	}
	for in, exp := range tests {
		testLayoutExt(t, l, in, exp)
	}
	l = extensions.NewLayoutTupleOmitPrefix()
	l.Delimiter = "edu/"
	l.TupleSize = 3
	l.TupleNum = 3
	l.Padding = "right"
	l.Reverse = false
	tests = map[string]string{
		"https://institution.edu/3448793":        "344/879/300/3448793",
		"https://institution.edu/abc/edu/f8.05v": "f8./05v/000/f8.05v",
	}
	for in, exp := range tests {
		testLayoutExt(t, l, in, exp)
	}

	// changing layout to an invalid state shouldn't affect previously
	// created layout funcs. check this doesn't panic
	id := "https://institution.edu/3448793"
	layoutFn, _ := l.NewFunc()
	l.TupleSize = 0 // invalid but no harm
	l.TupleNum = 0  // invalid but no harm
	p, err := layoutFn(id)
	if err != nil {
		t.Fatal(err)
	}
	if p != tests[id] {
		t.Fatalf("expected %s", tests[id])
	}
}
