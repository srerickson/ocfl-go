package ocfl

import (
	"testing"
)

func TestVersionHelpers(t *testing.T) {
	for _, n := range []string{"", "v0", "v00", "v", "1", "v.10", "v3.0", "asdf"} {
		v := VNum{}
		err := ParseVNum(n, &v)
		if err == nil {
			t.Errorf("parsing %s did not fail as expected", n)
		}
	}
	testVals := map[string][2]int{
		"v1":       {1, 0},
		"v100":     {100, 0},
		"v0000010": {10, 7},
		"v031":     {31, 3},
	}
	for key, val := range testVals {
		v := VNum{}
		err := ParseVNum(key, &v)
		if err != nil {
			t.Error(err)
		}
		if v.num != val[0] {
			t.Errorf("expected %s to parse to %d, got %d", key, val[0], v.num)
		}
		if v.padding != val[1] {
			t.Errorf("expected %s to parse to padding: %d, got padding %d", key, val[1], v.padding)
		}
	}
}

func TestValidVersionSeq(t *testing.T) {
	p := MustParseVNum
	// valid sequences
	valid := []VNums{
		{p("v1")},
		{p("v1"), p("v2"), p("v3"), p("v4"), p("v5")},
		{p("v001"), p("v002"), p("v003")},
	}
	for _, seq := range valid {
		err := seq.Valid()
		if err != nil {
			t.Error(err)
		}
	}
	// invalid sequenecs
	invalid := []VNums{
		{p("v2")},
		{p("v1"), p("v3"), p("v4"), p("v5")},
		{p("v01"), p("v02"), p("v03"), p("v04"), p("v05"), p("v06"), p("v07"), p("v08"), p("v09"), p("v10")},
	}
	for _, seq := range invalid {
		err := seq.Valid()
		if err == nil {
			t.Errorf("expected %v to be invalid", seq)
		}
	}

}
