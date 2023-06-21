package ocflv1_test

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend/local"
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
	t.Run("minimal stage", func(t *testing.T) {
		fsys := testfs.NewMemFS()
		alg := digest.SHA256()
		root := "object-root"
		id := "001"
		stage, err := ocfl.NewStage(alg, digest.Map{}, nil)
		if err != nil {
			t.Fatal(err)
		}
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

// documents using Commit() to make a new object based on last version state of another object.
func ExampleCommit_copyobject() {
	ctx := context.Background()
	//FS for reading source object
	fsys := ocfl.DirFS(filepath.Join(`..`, `testdata`, `object-fixtures`, `1.1`, `good-objects`))
	sourceObject, err := ocflv1.GetObject(ctx, fsys, `updates_all_actions`)
	if err != nil {
		log.Fatal("couldn't open source object:", err.Error())
	}
	state, err := sourceObject.ObjectState(ctx, 0)
	if err != nil {
		log.Fatal("couldn't retrieve logical state")
	}
	// prepare a place to write the new object
	dstPath, err := os.MkdirTemp("", "ocfl-commit-test-*")
	if err != nil {
		log.Fatal(err)
	}
	//log.Println(dstPath)
	defer os.RemoveAll(dstPath)
	writeFS, err := local.NewFS(dstPath)
	if err != nil {
		log.Fatal(err)
	}
	// commit
	// TODO: fix manifest for new object (to avoid 'v1/content/v2/content/...')
	stage, err := state.AsStage(true)
	if err != nil {
		log.Fatal(err)
	}
	err = ocflv1.Commit(ctx, writeFS, "object-copy", "object-001", stage)
	if err != nil {
		log.Fatal(err)
	}
	// read object back and validate
	cpObj, err := ocflv1.GetObject(ctx, writeFS, "object-copy")
	if err != nil {
		log.Fatal(err)
	}
	if result := cpObj.Validate(ctx); result.Err() != nil {
		log.Fatal("object is not valid: ", result.Err())
	}
	fmt.Println("object is valid")
	//Output: object is valid
}
