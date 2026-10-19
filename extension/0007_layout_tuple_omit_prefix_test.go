package extension_test

import (
	"errors"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/extension"
)

var _ (extension.Layout) = (*extension.LayoutTupleOmitPrefix)(nil)
var _ (extension.Extension) = (*extension.LayoutTupleOmitPrefix)(nil)

func TestLayoutTupleOmitPrefix(t *testing.T) {
	layout := extension.Ext0007().(*extension.LayoutTupleOmitPrefix)
	layout.TupleSize = 4
	layout.TupleNum = 2
	layout.Padding = "left"
	layout.Reverse = true
	tests := map[string]string{
		"namespace:12887296":                            "6927/8821/12887296",
		"urn:uuid:6e8bc430-9c3a-11d9-9669-0800200c9a66": "66a9/c002/6e8bc430-9c3a-11d9-9669-0800200c9a66",
		"abc123": "321c/ba00/abc123",
	}
	for in, exp := range tests {
		testLayoutExt(t, layout, in, exp)
	}
	layout = extension.Ext0007().(*extension.LayoutTupleOmitPrefix)
	layout.Delimiter = "edu/"
	layout.TupleSize = 3
	layout.TupleNum = 3
	layout.Padding = "right"
	layout.Reverse = false
	tests = map[string]string{
		"https://institution.edu/3448793":        "344/879/300/3448793",
		"https://institution.edu/abc/edu/f8.05v": "f8./05v/000/f8.05v",
	}
	for in, exp := range tests {
		testLayoutExt(t, layout, in, exp)
	}

	t.Run("ark with slash delimiter", func(t *testing.T) {
		layout := extension.Ext0007().(*extension.LayoutTupleOmitPrefix)
		layout.Delimiter = "/"
		testLayoutExt(t, layout, "ark:/13030/xyz", "000/000/xyz/xyz")
	})

	t.Run("invalid ids", func(t *testing.T) {
		defaultLayout := extension.Ext0007().(*extension.LayoutTupleOmitPrefix)
		reverseLayout := extension.Ext0007().(*extension.LayoutTupleOmitPrefix)
		reverseLayout.Reverse = true
		dotTuples := extension.Ext0007().(*extension.LayoutTupleOmitPrefix)
		dotTuples.TupleSize = 1
		dotTuples.TupleNum = 2
		dotTuples.Padding = "right"
		tests := map[string]struct {
			layout *extension.LayoutTupleOmitPrefix
			id     string
		}{
			"ark":                    {defaultLayout, "ark:/13030/xyz"},
			"ark nested in object":   {reverseLayout, "ark:/13030/xyz/v1/content/13030/xyz"},
			"leading slash":          {defaultLayout, "/abc"},
			"leading slash after ns": {defaultLayout, "ns:/abc"},
			"trailing slash":         {defaultLayout, "ns:abc/"},
			"dot-dot segments":       {defaultLayout, "a:/../../other/path"},
			"dot-dot":                {defaultLayout, "ns:.."},
			"dot":                    {defaultLayout, "ns:."},
			"bare dot-dot":           {defaultLayout, ".."},
			"empty":                  {defaultLayout, ""},
			"empty suffix":           {defaultLayout, "ns:"},
			"dot tuples":             {dotTuples, "ns:.x."},
			"dot-dot tuples":         {dotTuples, "ns:..x"},
		}
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := tc.layout.Resolve(tc.id)
				be.True(t, errors.Is(err, extension.ErrInvalidLayoutID))
				be.Equal(t, "", got)
			})
		}
	})

	t.Run("unmarshal", func(t *testing.T) {
		reg := extension.DefaultRegistry()
		ext, err := reg.Unmarshal([]byte(`{
			"delimiter": ":",
			"extensionName": "0007-n-tuple-omit-prefix-storage-layout",
			"tupleSize" : 4,
			"numberOfTuples" : 2,
			"zeroPadding" : "right",
			"reverseObjectRoot": true
		}`))
		be.NilErr(t, err)
		layout, ok := ext.(extension.Layout)
		be.True(t, ok)
		concretLayout, ok := layout.(*extension.LayoutTupleOmitPrefix)
		be.True(t, ok)
		be.Equal(t, ":", concretLayout.Delimiter)
		be.Equal(t, "0007-n-tuple-omit-prefix-storage-layout", concretLayout.Name())
		be.Equal(t, 4, concretLayout.TupleSize)
		be.Equal(t, 2, concretLayout.TupleNum)
		be.Equal(t, "right", concretLayout.Padding)
		be.Equal(t, true, concretLayout.Reverse)
	})
}
