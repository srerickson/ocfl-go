package ocflv1_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/testfs"
	"github.com/srerickson/ocfl/ocflv1"
)

// WriteFS with stage content for testing
func newCommitTestWriteFS(files map[string][]byte) (ocfl.WriteFS, error) {
	ctx := context.Background()
	fsys := testfs.NewMemFS()
	// stage1 commit is from storeFS, copy files
	for n, b := range files {
		_, err := fsys.Write(ctx, n, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
	}
	return fsys, nil
}

// tests for ocflv1.Commit():

func TestCommit(t *testing.T) {
	ctx := context.Background()
	t.Run("empty stage", func(t *testing.T) {
		fsys := testfs.NewMemFS()
		alg := digest.SHA256()
		root := "object-root"
		id := "001"
		stage := ocfl.NewStage(alg)
		if err := ocflv1.Commit(ctx, fsys, root, id, stage); err != nil {
			t.Fatal(err)
		}
		obj, result := ocflv1.ValidateObject(ctx, fsys, "object-root")
		if err := result.Err(); err != nil {
			t.Fatal(err)
		}
		if alg.ID() != obj.Algorithm {
			t.Fatal("expected digest to be", alg.ID())
		}
		if obj.Path != root {
			t.Fatal("expected object path to be", root)
		}
		inv, err := obj.Inventory(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if inv.ID != id {
			t.Fatal("expected object id to be", id)
		}
	})
}
