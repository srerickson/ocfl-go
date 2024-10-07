package extension_test

import (
	"encoding/json"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/extension"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

func testLayoutExt(t *testing.T, layout extension.Layout, in, expect string) {
	t.Helper()
	got, err := layout.Resolve(in)
	be.NilErr(t, err)
	be.Equal(t, expect, got)
}

func TestCustomLayout(t *testing.T) {
	reg := extension.NewRegistry(testutil.NewCustomLayout)
	name := testutil.NewCustomLayout().Name()
	layoutJSON := []byte(`{
		"extensionName": "` + name + `",
		"prefix": "test_objects"
	}`)
	ext, err := reg.Unmarshal(layoutJSON)
	be.NilErr(t, err)
	layout, ok := ext.(extension.Layout)
	be.True(t, ok)
	result, err := layout.Resolve("object-01")
	be.NilErr(t, err)
	expect := "test_objects/object-01"
	be.Equal(t, expect, result)
	// encode custom extension
	encJSON, err := json.Marshal(ext)
	be.NilErr(t, err)
	ext2, err := reg.Unmarshal(encJSON)
	be.NilErr(t, err)
	be.Equal(t, ext.Name(), ext2.Name())
}
