package ocfl_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/testfs"
)

func newTestFS(data map[string][]byte) ocfl.FS {
	ctx := context.Background()
	fsys := testfs.NewMemFS()
	for name, file := range data {
		_, err := fsys.Write(ctx, name, bytes.NewBuffer(file))
		if err != nil {
			panic(err)
		}
	}
	return fsys
}

func TestStage(t *testing.T) {
	ctx := context.Background()
	data := map[string][]byte{
		"dir/README.txt": []byte("README content"),
		"file.txt":       []byte("same file content"),
		"file2.txt":      []byte("same file content"),
	}
	stage, err := ocfl.NewStage(digest.SHA256(), digest.Map{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	stage.FS = newTestFS(data)
	if err := stage.AddRoot(ctx, "."); err != nil {
		t.Fatal(err)
	}
	state := stage.State()
	for name := range data {
		if state.GetDigest(name) == "" {
			t.Fatalf("state from stage does not included '%s' as expected", name)
		}
	}
}
