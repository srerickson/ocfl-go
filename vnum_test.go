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

func TestVNum_First(t *testing.T) {
	tests := []struct {
		name    string
		vnum    VNum
		isFirst bool
	}{
		{"v1 is first", MustParseVNum("v1"), true},
		{"v001 is first", MustParseVNum("v001"), true},
		{"v2 is not first", MustParseVNum("v2"), false},
		{"v10 is not first", MustParseVNum("v10"), false},
		{"zero value is not first", VNum{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.vnum.First()
			if result != tt.isFirst {
				t.Errorf("expected First() = %v, got %v for %s", tt.isFirst, result, tt.vnum.String())
			}
		})
	}
}

func TestVNum_Next(t *testing.T) {
	tests := []struct {
		name    string
		vnum    VNum
		wantNum int
		wantPad int
		wantErr bool
	}{
		{"next of v1", MustParseVNum("v1"), 2, 0, false},
		{"next of v5", MustParseVNum("v5"), 6, 0, false},
		{"next of v001", MustParseVNum("v001"), 2, 3, false},
		{"next of v098", MustParseVNum("v098"), 99, 3, false},
		{"next of v099 with padding 3 overflows", MustParseVNum("v099"), 0, 0, true},
		{"next of v9 with padding 1 overflows", V(9, 1), 0, 0, true},
		{"next of v99 with padding 2 overflows", V(99, 2), 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, err := tt.vnum.Next()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for Next() of %v", tt.vnum)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if next.num != tt.wantNum {
					t.Errorf("expected num=%d, got %d", tt.wantNum, next.num)
				}
				if next.padding != tt.wantPad {
					t.Errorf("expected padding=%d, got %d", tt.wantPad, next.padding)
				}
			}
		})
	}
}

func TestVNum_MarshalText(t *testing.T) {
	tests := []struct {
		name    string
		vnum    VNum
		want    string
		wantErr bool
	}{
		{"v1 marshals", MustParseVNum("v1"), "v1", false},
		{"v10 marshals", MustParseVNum("v10"), "v10", false},
		{"v001 marshals with padding", MustParseVNum("v001"), "v001", false},
		{"v042 marshals with padding", MustParseVNum("v042"), "v042", false},
		{"zero value fails", VNum{}, "", true},
		{"invalid padding fails", VNum{num: 100, padding: 2}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.vnum.MarshalText()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error marshaling %v", tt.vnum)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if string(data) != tt.want {
					t.Errorf("expected %q, got %q", tt.want, string(data))
				}
			}
		})
	}
}

func TestVNum_UnmarshalText(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantNum int
		wantPad int
		wantErr bool
	}{
		{"v1 unmarshals", "v1", 1, 0, false},
		{"v100 unmarshals", "v100", 100, 0, false},
		{"v001 unmarshals", "v001", 1, 3, false},
		{"v042 unmarshals", "v042", 42, 3, false},
		{"invalid format fails", "1", 0, 0, true},
		{"missing v prefix fails", "42", 0, 0, true},
		{"empty string fails", "", 0, 0, true},
		{"v0 fails", "v0", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var vnum VNum
			err := vnum.UnmarshalText([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error unmarshaling %q", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if vnum.num != tt.wantNum {
					t.Errorf("expected num=%d, got %d", tt.wantNum, vnum.num)
				}
				if vnum.padding != tt.wantPad {
					t.Errorf("expected padding=%d, got %d", tt.wantPad, vnum.padding)
				}
			}
		})
	}
}

func TestVNum_MarshalUnmarshal_RoundTrip(t *testing.T) {
	tests := []string{"v1", "v10", "v001", "v042", "v100"}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			original := MustParseVNum(input)

			// Marshal
			data, err := original.MarshalText()
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			// Unmarshal
			var result VNum
			err = result.UnmarshalText(data)
			if err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

			// Should match
			if result.num != original.num || result.padding != original.padding {
				t.Errorf("round trip failed: original=%v, result=%v", original, result)
			}
		})
	}
}

func TestVNum_Lineage(t *testing.T) {
	tests := []struct {
		name     string
		vnum     VNum
		wantLen  int
		wantHead VNum
	}{
		{"v1 lineage", MustParseVNum("v1"), 1, MustParseVNum("v1")},
		{"v5 lineage", MustParseVNum("v5"), 5, MustParseVNum("v5")},
		{"v001 lineage", MustParseVNum("v001"), 1, MustParseVNum("v001")},
		{"v010 lineage", MustParseVNum("v010"), 10, MustParseVNum("v010")},
		{"zero value lineage", VNum{}, 0, VNum{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lineage := tt.vnum.Lineage()

			if len(lineage) != tt.wantLen {
				t.Errorf("expected length %d, got %d", tt.wantLen, len(lineage))
			}

			if tt.wantLen > 0 {
				head := lineage.Head()
				if head.num != tt.wantHead.num || head.padding != tt.wantHead.padding {
					t.Errorf("expected head %v, got %v", tt.wantHead, head)
				}

				// Verify lineage is valid sequence
				err := lineage.Valid()
				if err != nil {
					t.Errorf("lineage should be valid: %v", err)
				}

				// Verify all versions have same padding
				for i, v := range lineage {
					if v.num != i+1 {
						t.Errorf("lineage[%d] should have num=%d, got %d", i, i+1, v.num)
					}
					if v.padding != tt.vnum.padding {
						t.Errorf("lineage[%d] should have padding=%d, got %d", i, tt.vnum.padding, v.padding)
					}
				}
			}
		})
	}
}

func TestVNums_Padding(t *testing.T) {
	tests := []struct {
		name    string
		vnums   VNums
		wantPad int
	}{
		{"no padding", VNums{MustParseVNum("v1"), MustParseVNum("v2")}, 0},
		{"padding 3", VNums{MustParseVNum("v001"), MustParseVNum("v002")}, 3},
		{"empty vnums", VNums{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pad := tt.vnums.Padding()
			if pad != tt.wantPad {
				t.Errorf("expected padding %d, got %d", tt.wantPad, pad)
			}
		})
	}
}
