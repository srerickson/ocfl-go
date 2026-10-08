package ocfl_test

import (
	"context"
	"encoding/json"
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
)

var (
	objectFixturesPath = filepath.Join(`testdata`, `object-fixtures`)
	storeFixturePath   = filepath.Join(`testdata`, `store-fixtures`)
)

// stageBytes clears stage and adds the files in content to it.
func stageBytes(t *testing.T, stage *ocfl.Stage, content map[string][]byte, fixity ...digest.Algorithm) *ocfl.Stage {
	t.Helper()
	be.NilErr(t, stage.Clear())
	for name, b := range content {
		be.NilErr(t, stage.AddBytes(name, b, fixity...))
	}
	return stage
}

// commit finalizes the stage's update and applies it to obj's storage,
// returning the updated object.
func commit(t *testing.T, obj *ocfl.Object, stage *ocfl.Stage, msg string, opts ...ocfl.UpdateOption) *ocfl.Object {
	t.Helper()
	ctx := context.Background()
	be.NilErr(t, stage.Finalize(msg, ocfl.User{Name: "Tester"}, opts...))
	newObj, err := stage.Update().Apply(ctx, obj.FS(), obj.Path(), stage.Content())
	be.NilErr(t, err)
	return newObj
}

// sortedKeys returns the sorted keys in m.
func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}

// mustMarshal returns v encoded as JSON.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	be.NilErr(t, err)
	return b
}
