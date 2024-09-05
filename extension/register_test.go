package extension_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/srerickson/ocfl-go/extension"
)

func TestUnmarshalJSON(t *testing.T) {
	reg := extension.DefaultRegister()
	names := reg.Names()
	if len(names) == 0 {
		t.Fatal("expected > 0 registered extensions")
	}
	for _, name := range names {
		_, err := reg.Unmarshal([]byte(`{"extensionName": "` + name + `"}`))
		if err != nil {
			t.Error("Unmarshal()", name, err)
		}
	}
}

func TestMarshalJSON(t *testing.T) {
	// Test json encoding for all registered extensions.
	reg := extension.DefaultRegister()
	for _, name := range reg.Names() {
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
