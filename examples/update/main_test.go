package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/fs/local"
	"github.com/srerickson/ocfl-go/stage"
)

var contentFixture = filepath.Join("..", "..", "testdata", "content-fixture")

func TestRun(t *testing.T) {
	ctx := context.Background()
	t.Run("new object", func(t *testing.T) {
		objDir := t.TempDir()
		stageFile := filepath.Join(t.TempDir(), "stage.json")
		var stderr bytes.Buffer
		err := run(ctx, []string{
			"-obj", objDir, "-src", contentFixture, "-id", "ark:/123",
			"-msg", "first", "-name", "Tester", "-email", "tester@example.org",
			"-stage", stageFile,
		}, &stderr)
		be.NilErr(t, err)
		checkObject(t, objDir, "ark:/123")
		// the stage file is removed once the update is applied
		_, err = os.Stat(stageFile)
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})
	t.Run("resume from stage file", func(t *testing.T) {
		objDir := t.TempDir()
		objFS := local.MustNewFS(objDir)
		update, _, err := ocfl.NewObjectUpdate(ctx, objFS, ".", "ark:/123")
		be.NilErr(t, err)
		staged := &stagedUpdate{Update: update, Content: &stage.FileContentMap{}}
		be.NilErr(t, stage.FS(ctx, staged.Update, staged.Content, local.MustNewFS(contentFixture), ".", "."))
		be.NilErr(t, staged.Update.Finalize("first", ocfl.User{Name: "Tester"}))
		stageFile := filepath.Join(t.TempDir(), "stage.json")
		data, err := json.Marshal(staged)
		be.NilErr(t, err)
		be.NilErr(t, os.WriteFile(stageFile, data, 0o644))

		var stderr bytes.Buffer
		be.NilErr(t, run(ctx, []string{"-obj", objDir, "-stage", stageFile}, &stderr))
		checkObject(t, objDir, "ark:/123")
		_, err = os.Stat(stageFile)
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})
	t.Run("incomplete stage file", func(t *testing.T) {
		for _, data := range []string{
			`{"update":null,"content":null}`,
			`{"content":{"sources":[],"content":{}}}`,
			`{}`,
		} {
			stageFile := filepath.Join(t.TempDir(), "stage.json")
			be.NilErr(t, os.WriteFile(stageFile, []byte(data), 0o644))
			var stderr bytes.Buffer
			err := run(ctx, []string{"-obj", t.TempDir(), "-stage", stageFile}, &stderr)
			be.Nonzero(t, err)
			be.In(t, "stage must have an update and content", err.Error())
		}
	})
	t.Run("missing flags", func(t *testing.T) {
		var stderr bytes.Buffer
		err := run(ctx, []string{"-obj", t.TempDir(), "-src", contentFixture}, &stderr)
		be.Nonzero(t, err)
		be.In(t, "missing required flags: email, msg, name", stderr.String())
	})
}

// checkObject checks that a valid object with the id and one version is in
// dir.
func checkObject(t *testing.T, dir, id string) {
	t.Helper()
	ctx := context.Background()
	objFS := local.MustNewFS(dir)
	be.NilErr(t, ocfl.ValidateObject(ctx, objFS, ".").Err())
	obj, err := ocfl.NewObject(ctx, objFS, ".")
	be.NilErr(t, err)
	be.Equal(t, id, obj.ID())
	be.Equal(t, 1, obj.Head().Num())
}
