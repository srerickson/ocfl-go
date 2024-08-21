package extension_test

import (
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/extension"
)

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

	t.Run("unmarshal", func(t *testing.T) {
		ext, err := extension.Unmarshal([]byte(`{
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
