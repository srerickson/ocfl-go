package ocflv1_test

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend/local"
	"github.com/srerickson/ocfl/backend/memfs"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/ocflv1"
)

func TestCommit(t *testing.T) {
	t.Run("minimal stage", func(t *testing.T) {
		ctx := context.Background()
		fsys := memfs.New()
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
	t.Run("update fixture", func(t *testing.T) {
		ctx := context.Background()
		fixtures := filepath.Join(`..`, `testdata`, `object-fixtures`, `1.1`)
		fsys := ocfl.DirFS(fixtures)
		runTestsFn := func(obj *ocflv1.Object) error {
			t.Run(obj.Path, func(t *testing.T) {
				testUpdateObject(ctx, obj, t)
			})
			return nil
		}
		// add all version state of all good objects to states
		if err := ocflv1.ScanObjects(ctx, fsys, "good-objects", runTestsFn, nil); err != nil {
			t.Fatal(err)
		}
		// add all versions state of all warn objects to stattes
		if err := ocflv1.ScanObjects(ctx, fsys, "warn-objects", runTestsFn, nil); err != nil {
			t.Fatal(err)
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

func testUpdateObject(ctx context.Context, fixtureObj *ocflv1.Object, t *testing.T) {
	tmpdir, err := mkObjectTemp(fixtureObj)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	// writable FS for tmpdir
	writeFS, err := local.NewFS(tmpdir)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := ocflv1.GetObject(ctx, writeFS, fixtureObj.Path)
	if err != nil {
		t.Fatal(err)
	}
	state, err := obj.ObjectState(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := obj.Inventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// new content from in-memory FS
	newContent := map[string]io.Reader{
		"testdata/delete.txt":     strings.NewReader("This file will be deleted"),
		"testdata/rename-src.txt": strings.NewReader("This file will be renamed"),
		"testdata/updated.txt":    strings.NewReader("This file will be updated"),
		"testdata/unchanged.txt":  strings.NewReader("This file will be unchanged"),
	}
	newContentFS, err := memfs.NewWith(newContent)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := ocfl.NewStage(state.Alg, state.Map, newContentFS)
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.AddRoot(ctx, ".", digest.MD5()); err != nil {
		t.Fatal(err)
	}
	if err := ocflv1.Commit(ctx, writeFS, obj.Path, inv.ID, stage); err != nil {
		t.Fatal(err)
	}

	// validite
	obj2, result := ocflv1.ValidateObject(ctx, writeFS, obj.Path)
	if err := result.Err(); err != nil {
		t.Fatal(err)
	}
	state2, err := obj2.ObjectState(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := state2.ReadDir(ctx, "testdata")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatal("expected 4 entries in 'testdata'")
	}
	// TODO: commit with remove, rename, update
}

// creates a temporary directory and copies all files in the object into the
// directory, returning the tmp directory root. The object will be located at
// obj.Path in the new tmp directory. The caller should remember to removeall
// the temp directoy.
func mkObjectTemp(obj *ocflv1.Object) (string, error) {
	ctx := context.Background()
	tmpdir, err := os.MkdirTemp("", "test-ocfl-object-*")
	if err != nil {
		return "", err
	}
	err = ocfl.EachFile(ctx, obj.FS, obj.Path, func(name string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dir := path.Dir(name)
		if err := os.MkdirAll(filepath.Join(tmpdir, filepath.FromSlash(dir)), 0777); err != nil {
			return err
		}
		f, err := obj.FS.OpenFile(ctx, name)
		if err != nil {
			return err
		}
		defer f.Close()
		w, err := os.Create(filepath.Join(tmpdir, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, f); err != nil {
			return err
		}
		return w.Sync()
	})
	if err != nil {
		return "", err
	}
	return tmpdir, nil
}
