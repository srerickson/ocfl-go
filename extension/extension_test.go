package extension_test

import (
	"testing"

	"github.com/srerickson/ocfl-go/extension"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

func testLayoutExt(t *testing.T, layout extension.Layout, in, out string) {
	t.Helper()
	got, err := layout.Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("expected %s, got %s", out, got)
	}
}

func TestCustomLayout(t *testing.T) {
	reg := extension.NewRegister(testutil.NewCustomLayout)
	name := testutil.NewCustomLayout().Name()
	layoutJSON := []byte(`{
		"extensionName": "` + name + `",
		"prefix": "test_objects"
	}`)
	ext, err := reg.Unmarshal(layoutJSON)
	if err != nil {
		t.Fatal("extension.UnmarshalLayout()", err)
	}
	layout, ok := ext.(extension.Layout)
	if !ok {
		t.Fatal("not a layout")
	}
	result, err := layout.Resolve("object-01")
	if err != nil {
		t.Fatal("Resolve()", err)
	}
	expect := "test_objects/object-01"
	if result != expect {
		t.Errorf("Resolve(): got=%q, expected=%q", result, expect)
	}
}
