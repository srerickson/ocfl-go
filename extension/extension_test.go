package extension_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/srerickson/ocfl-go/extension"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

func TestUnmarshalJSON(t *testing.T) {
	names := extension.Registered()
	if len(names) == 0 {
		t.Fatal("expected > 0 registered extensions")
	}
	for _, name := range names {
		_, err := extension.Unmarshal([]byte(`{"extensionName": "` + name + `"}`))
		if err != nil {
			t.Error("Unmarshal()", name, err)
		}
	}
}

func TestMarshalJSON(t *testing.T) {
	// Test json encoding for all registered extensions.
	for _, name := range extension.Registered() {
		ext, err := extension.Get(name)
		if err != nil {
			t.Error("Get()", name, err)
			continue
		}
		extJson, err := json.Marshal(ext)
		if err != nil {
			t.Error("json.Marshal()", name, err)
			continue
		}
		// check extensionName
		vals := map[string]any{}
		if err := json.Unmarshal(extJson, &vals); err != nil {
			t.Error("json.Unmarshal() into map", name, err)
			continue
		}
		val, hasVal := vals["extensionName"]
		extName, isStr := val.(string)
		if !hasVal {
			t.Error("json encoding for", name, "doesn't include extensionName")
			continue
		}
		if !isStr {
			t.Error("json encoding for", name, "has non-string extensionName")
			continue
		}
		if extName != name {
			t.Error("json encoding for", name, "has incorrect extensionName")
			continue
		}
		// round-trip for json encoding should yield equal ext.
		duplicate, err := extension.Get(name)
		if err != nil {
			t.Error("Get()", name, err)
			continue
		}
		if err := json.Unmarshal(extJson, duplicate); err != nil {
			t.Error("json.Unmarshal() into extension:", err)
			continue
		}
		if !reflect.DeepEqual(ext, duplicate) {
			t.Error("round-trip encoding resulted in different values for", name)
		}

	}
}

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
	extension.Register(testutil.NewCustomLayout)
	name := testutil.NewCustomLayout().Name()
	layoutJSON := []byte(`{
		"extensionName": "` + name + `",
		"prefix": "test_objects"
	}`)
	ext, err := extension.Unmarshal(layoutJSON)
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
