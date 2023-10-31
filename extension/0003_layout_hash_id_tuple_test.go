package extension_test

import (
	"testing"

	"github.com/srerickson/ocfl-go/extension"
)

func TestLayoutHashIDTuple(t *testing.T) {
	{
		layout := extension.Ext0003().(*extension.LayoutHashIDTuple)
		table := map[string]string{
			`object-01`:        `3c0/ff4/240/object-01`,
			`..hor/rib:le-$id`: `487/326/d8c/%2e%2ehor%2frib%3ale-%24id`,
			`..Hor/rib:lè-$id`: `373/529/21a/%2e%2eHor%2frib%3al%c3%a8-%24id`,
		}
		for in, exp := range table {
			testLayoutExt(t, layout, in, exp)
		}
	}
	{
		layout := extension.LayoutHashIDTuple{}
		layout.DigestAlgorithm = "md5"
		layout.TupleSize = 2
		layout.TupleNum = 15
		table := map[string]string{
			"object-01":        "ff/75/53/44/92/48/5e/ab/b3/9f/86/35/67/28/88/object-01",
			"..hor/rib:le-$id": "08/31/97/66/fb/6c/29/35/dd/17/5b/94/26/77/17/%2e%2ehor%2frib%3ale-%24id",
		}
		for in, exp := range table {
			testLayoutExt(t, layout, in, exp)
		}
	}
	{
		// 4-tuple
		l := extension.Ext0003().(*extension.LayoutHashIDTuple)
		l.TupleNum = 4
		table := map[string]string{
			`ark:123/abc`: `a47/817/83d/cec/ark%3a123%2fabc`,
		}
		for in, exp := range table {
			testLayoutExt(t, l, in, exp)
		}
	}

}
