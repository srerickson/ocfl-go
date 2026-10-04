package ocfl_test

import (
	"context"
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

// stageBytes clears u's state and returns a stage with the files in content.
func stageBytes(t *testing.T, u *ocfl.ObjectUpdate, content map[string][]byte, fixity ...digest.Algorithm) *ocfl.Stage {
	t.Helper()
	be.NilErr(t, u.Clear())
	stage := ocfl.NewStage(u)
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
	be.NilErr(t, stage.Update.Finalize(msg, ocfl.User{Name: "Tester"}, opts...))
	newObj, err := stage.Update.Apply(ctx, obj.FS(), obj.Path(), stage.Content)
	be.NilErr(t, err)
	return newObj
}

// sortedKeys returns the sorted keys in m.
func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
