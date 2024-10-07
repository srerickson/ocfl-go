package extension_test

import (
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/extension"
)

var _ (extension.Layout) = (*extension.LayoutHashIDTuple)(nil)
var _ (extension.Extension) = (*extension.LayoutHashIDTuple)(nil)

func TestLayoutHashIDTuple(t *testing.T) {
	t.Run("defualt values", func(t *testing.T) {
		layout := extension.Ext0003().(*extension.LayoutHashIDTuple)
		table := map[string]string{
			`object-01`:        `3c0/ff4/240/object-01`,
			`..hor/rib:le-$id`: `487/326/d8c/%2e%2ehor%2frib%3ale-%24id`,
			`..Hor/rib:lè-$id`: `373/529/21a/%2e%2eHor%2frib%3al%c3%a8-%24id`,
		}
		be.Nonzero(t, layout.Doc())
		be.Equal(t, "0003-hash-and-id-n-tuple-storage-layout", layout.Name())
		for in, exp := range table {
			testLayoutExt(t, layout, in, exp)
		}
	})
	t.Run("custom values", func(t *testing.T) {
		layout := extension.LayoutHashIDTuple{
			DigestAlgorithm: "md5",
			TupleSize:       2,
			TupleNum:        15,
		}
		table := map[string]string{
			"object-01":        "ff/75/53/44/92/48/5e/ab/b3/9f/86/35/67/28/88/object-01",
			"..hor/rib:le-$id": "08/31/97/66/fb/6c/29/35/dd/17/5b/94/26/77/17/%2e%2ehor%2frib%3ale-%24id",
		}
		for in, exp := range table {
			testLayoutExt(t, layout, in, exp)
		}
	})
	t.Run("default: 4-tuple", func(t *testing.T) {
		l := extension.Ext0003().(*extension.LayoutHashIDTuple)
		l.TupleNum = 4
		table := map[string]string{
			`ark:123/abc`: `a47/817/83d/cec/ark%3a123%2fabc`,
		}
		for in, exp := range table {
			testLayoutExt(t, l, in, exp)
		}
	})
}
