package ocflv1_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/local"
	"github.com/srerickson/ocfl-go/backend/memfs"
	"github.com/srerickson/ocfl-go/ocflv1"
	"golang.org/x/exp/maps"
)

func TestCommit(t *testing.T) {
	t.Run("minimal stage", func(t *testing.T) {
		ctx := context.Background()
		fsys := memfs.New()
		alg := ocfl.SHA256
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
		if alg.ID() != obj.SidecarAlg {
			t.Fatal("expected digest to be", alg.ID())
		}
		if obj.Path != root {
			t.Fatal("expected object path to be", root)
		}
		if err := obj.SyncInventory(ctx); err != nil {
			t.Fatal(err)
		}
		if obj.Inventory.ID != id {
			t.Fatal("expected object id to be", id)
		}
	})
	t.Run("allow unchanged", func(t *testing.T) {
		ctx := context.Background()
		fsys, err := memfs.NewWith(map[string]io.Reader{
			"file.txt": strings.NewReader("content"),
		})
		if err != nil {
			t.Fatal(err)
		}
		alg := ocfl.SHA256
		root := "object-root"
		id := "001"
		stage := ocfl.NewStage(alg)
		if err := stage.AddFS(ctx, fsys, "."); err != nil {
			t.Fatal("Commit() test setup:", err)
		}
		if err := ocflv1.Commit(ctx, fsys, root, id, stage); err != nil {
			t.Fatal("Commit() test setup:", err)
		}
		err = ocflv1.Commit(ctx, fsys, root, id, stage)
		if err == nil {
			t.Error("Commit() should return an error because of duplicate version")
		}
		err = ocflv1.Commit(ctx, fsys, root, id, stage, ocflv1.WithAllowUnchanged())
		if err != nil {
			t.Error("Commit() didn't allow unchanged version with WithAllowUnchange", err)
		}
	})
	t.Run("update fixture", func(t *testing.T) {
		ctx := context.Background()
		fixtures := filepath.Join(`..`, `testdata`, `object-fixtures`, `1.1`)
		fsys := ocfl.DirFS(fixtures)
		runTestsFn := func(objRoot *ocfl.ObjectRoot) error {
			t.Run(objRoot.Path, func(t *testing.T) {
				testUpdateObject(ctx, objRoot, t)
			})
			return nil
		}
		// add all version state of all good objects to states
		if err := ocfl.ObjectRoots(ctx, fsys, ocfl.Dir("good-objects"), runTestsFn); err != nil {
			t.Fatal(err)
		}
		// add all versions state of all warn objects to stattes
		if err := ocfl.ObjectRoots(ctx, fsys, ocfl.Dir("warn-objects"), runTestsFn); err != nil {
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
	state, err := sourceObject.State(0)
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
	// construct a stage using the object's state, manifest and fixity.
	stage := ocfl.NewStage(state.Alg)
	stage.State = state.DigestMap
	stage.SetFS(sourceObject.FS, sourceObject.Path)
	err = stage.UnsafeSetManifestFixty(sourceObject.Inventory.Manifest, sourceObject.Inventory.Fixity)
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

func testUpdateObject(ctx context.Context, fixtureObj *ocfl.ObjectRoot, t *testing.T) {
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
	originalState, err := obj.State(0)
	if err != nil {
		t.Fatal(err)
	}
	originalAlg := originalState.Alg
	t.Run("invalid-id", func(t *testing.T) {
		stage := ocfl.NewStage(obj.Inventory.DigestAlgorithm)
		badID := "wrong"
		err := ocflv1.Commit(ctx, writeFS, obj.Path, badID, stage)
		if err == nil {
			t.Error("Commit() didn't return error for invalid object id")
		}
	})
	t.Run("invalid-objpath-existing", func(t *testing.T) {
		stage := ocfl.NewStage(obj.Inventory.DigestAlgorithm)
		badPath := path.Dir(obj.Path)
		err := ocflv1.Commit(ctx, writeFS, badPath, obj.Inventory.ID, stage)
		if err == nil {
			t.Error("Commit() didn't return error for invalid object path")
		}
	})
	t.Run("invalid-objpath-isfile", func(t *testing.T) {
		stage := ocfl.NewStage(obj.Inventory.DigestAlgorithm)
		badPath := path.Join(obj.Path, "inventory.json")
		err := ocflv1.Commit(ctx, writeFS, badPath, obj.Inventory.ID, stage)
		if err == nil {
			t.Error("Commit() didn't return error for invalid object path")
		}
	})
	t.Run("invalid-head", func(t *testing.T) {
		stage := ocfl.NewStage(obj.Inventory.DigestAlgorithm)
		badHead := obj.Inventory.Head.Num()
		err := ocflv1.Commit(ctx, writeFS, obj.Path, obj.Inventory.ID, stage, ocflv1.WithHEAD(badHead))
		if err == nil {
			t.Error("Commit() didn't return error for invalid option WithHEAD value")
		}
	})
	t.Run("invalid-spec", func(t *testing.T) {
		stage := ocfl.NewStage(obj.Inventory.DigestAlgorithm)
		// test fixture use ocfl v1.1
		err := ocflv1.Commit(ctx, writeFS, obj.Path, obj.Inventory.ID, stage, ocflv1.WithOCFLSpec(ocfl.Spec1_0))
		if err == nil {
			t.Error("Commit() didn't return error for invalid option WithOCFLSpec value")
		}
	})
	t.Run("invalid-alg", func(t *testing.T) {
		alg := ocfl.SHA256
		if originalAlg == ocfl.SHA256 {
			alg = ocfl.SHA512
		}
		// test fixture use ocfl v1.1
		err := ocflv1.Commit(ctx, writeFS, obj.Path, obj.Inventory.ID, ocfl.NewStage(alg))
		if err == nil {
			t.Error("Commit() didn't return error for stage with different alg")
		}
	})
	t.Run("update-1", func(t *testing.T) {
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
		stage := ocfl.NewStage(originalAlg)
		stage.State = originalState.DigestMap
		if err := stage.AddFS(ctx, newContentFS, ".", ocfl.MD5); err != nil {
			t.Fatal(err)
		}
		if err := ocflv1.Commit(ctx, writeFS, obj.Path, obj.Inventory.ID, stage); err != nil {
			t.Fatal(err)
		}
		// validite
		updatedObj, result := ocflv1.ValidateObject(ctx, writeFS, obj.Path)
		if err := result.Err(); err != nil {
			t.Fatal(err)
		}
		updatedState, err := updatedObj.State(0)
		if err != nil {
			t.Fatal(err)
		}
		// check that new inventory has new content in fixity
		md5fixity := updatedObj.Inventory.Fixity[ocfl.MD5]
		if len(md5fixity.Digests()) == 0 {
			t.Fatal("inventory should have md5 block in fixity")
		}
		// expected state paths
		expectedPaths := append(originalState.Paths(), maps.Keys(newContent)...)
		// check that expected paths exist
		for _, name := range expectedPaths {
			dig := updatedState.GetDigest(name)
			if dig == "" {
				t.Fatal("missing path in updated state:", name)
			}
			if _, ok := newContent[name]; !ok {
				continue
			}
			for _, p := range updatedState.Manifest.DigestPaths(dig) {
				if md5fixity.GetDigest(p) == "" {
					t.Fatal("missing path in updated fixity", name)
				}
			}
		}
	})

	t.Run("update-2", func(t *testing.T) {
		if err := obj.SyncInventory(ctx); err != nil {
			t.Fatal(err)
		}
		state, err := obj.State(0)
		if err != nil {
			t.Fatal(err)
		}
		newContentFS, err := memfs.NewWith(map[string]io.Reader{
			"testdata/updated.txt": strings.NewReader("This is updated content"),
		})
		if err != nil {
			t.Fatal(err)
		}
		stage := ocfl.NewStage(state.Alg)
		stage.State = state.DigestMap
		if err := stage.AddFS(ctx, newContentFS, "."); err != nil {
			t.Fatal(err)
		}
		stage.State, err = stage.State.Remap(ocfl.Remove("testdata/delete.txt"))
		if err != nil {
			t.Fatal(err)
		}
		stage.State, err = stage.State.Remap(ocfl.Rename("testdata/rename-src.txt", "testdata/rename-dst.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if err := ocflv1.Commit(ctx, writeFS, obj.Path, obj.Inventory.ID, stage); err != nil {
			t.Fatal(err)
		}
		updatedObj, result := ocflv1.ValidateObject(ctx, writeFS, obj.Path)
		if err := result.Err(); err != nil {
			t.Fatal(err)
		}
		updatedState, err := updatedObj.State(0)
		if err != nil {
			t.Fatal(err)
		}
		if updatedState.GetDigest("testdata/delete.txt") != "" {
			t.Fatal("expected 'testdata/delete.txt' to be removed")
		}
		if updatedState.GetDigest("testdata/rename-src.txt") != "" {
			t.Fatal("expected 'testdata/rename-src.txt' to be removed")
		}
		if updatedState.GetDigest("testdata/rename-dst.txt") == "" {
			t.Fatal("expected 'testdata/rename-dst.txt' to exist")
		}
		// check updated path
		prevState, err := updatedObj.State(updatedObj.Inventory.Head.Num() - 1)
		if err != nil {
			t.Fatal(err)
		}
		updatedPath := "testdata/updated.txt"
		prevDigest := prevState.GetDigest(updatedPath)
		if prevDigest == updatedState.GetDigest(updatedPath) {
			t.Fatalf("expected '%s' to have changed", updatedPath)
		}
	})

}

// creates a temporary directory and copies all files in the object into the
// directory, returning the tmp directory root. The object will be located at
// obj.Path in the new tmp directory. The caller should remember to removeall
// the temp directoy.
func mkObjectTemp(obj *ocfl.ObjectRoot) (string, error) {
	ctx := context.Background()
	tmpdir, err := os.MkdirTemp("", "test-ocfl-object-*")
	if err != nil {
		return "", err
	}
	err = ocfl.Files(ctx, obj.FS, ocfl.Dir(obj.Path), func(name string) error {
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
