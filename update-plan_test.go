package ocfl_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/fs/local"
)

func TestUpdatePlan_Marshal(t *testing.T) {
	// create an update
	ctx := context.Background()
	fixture := filepath.Join(`testdata`, `object-fixtures`, `1.0`, `good-objects`)
	fsys, err := local.NewFS(fixture)
	be.NilErr(t, err)
	obj, err := ocfl.NewObject(ctx, fsys, "spec-ex-full")
	be.NilErr(t, err)
	stage, err := ocfl.StageBytes(map[string][]byte{}, digest.SHA512)
	be.NilErr(t, err)
	update, err := obj.NewUpdate(stage, "new version", ocfl.User{Name: "Me"})
	be.NilErr(t, err)
	be.Nonzero(t, len(update.Steps))
	updateBytes, err := update.Marshal()
	be.NilErr(t, err)
	sameUpdate, err := ocfl.RecoverUpdatePlan(updateBytes, fsys, "spec-ex-full", stage)
	be.NilErr(t, err)
	be.True(t, bytes.Equal(update.NewInventoryBytes, sameUpdate.NewInventoryBytes))
	be.True(t, bytes.Equal(update.OldInventoryBytes, sameUpdate.OldInventoryBytes))
	be.True(t, update.Steps.Eq(sameUpdate.Steps))
}
