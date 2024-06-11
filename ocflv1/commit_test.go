package ocflv1_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/local"
	"github.com/srerickson/ocfl-go/backend/memfs"
	"github.com/srerickson/ocfl-go/ocflv1"
)

func TestCommit(t *testing.T) {
	t.Run("minimal stage", func(t *testing.T) {
		ctx := context.Background()
		fsys := memfs.New()
		alg := ocfl.SHA256
		root := "object-root"
		id := "001"
		stage := &ocfl.Stage{State: ocfl.DigestMap{}, DigestAlgorithm: alg}
		if err := ocflv1.Commit(ctx, fsys, root, id, stage); err != nil {
			t.Fatal(err)
		}
		obj, result := ocflv1.ValidateObject(ctx, fsys, "object-root")
		if err := result.Err(); err != nil {
			t.Fatal(err)
		}
		if alg != obj.State.SidecarAlg {
			t.Fatal("expected digest to be", alg)
		}
		if obj.Path != root {
			t.Fatal("expected object path to be", root)
		}
		if err := obj.ReadInventory(ctx); err != nil {
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
		alg := `sha256`
		root := "object-root"
		id := "001"
		stage, err := ocfl.StageDir(ctx, fsys, ".", alg)
		if err != nil {
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
			t.Error("Commit() didn't allow unchanged version with WithAllowUnchange:", err)
		}
	})
	t.Run("update fixture", func(t *testing.T) {
		ctx := context.Background()
		fixtures := filepath.Join(`..`, `testdata`, `object-fixtures`, `1.1`)
		fsys := ocfl.DirFS(fixtures)
		runTestsFn := func(objRoot *ocfl.ObjectRoot, err error) bool {
			if err != nil {
				t.Error(err)
				return false
			}
			t.Run(objRoot.Path, func(t *testing.T) {
				testUpdateObject(ctx, objRoot, t)
			})
			return true
		}
		// add all version state of all good objects to states
		ocfl.ObjectRoots(ctx, fsys, "good-objects")(runTestsFn)
		// add all versions state of all warn objects to stattes
		ocfl.ObjectRoots(ctx, fsys, "warn-objects")(runTestsFn)
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
	ver := sourceObject.Inventory.Version(0)
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
	stage := &ocfl.Stage{
		DigestAlgorithm: sourceObject.Inventory.DigestAlgorithm,
		State:           ver.State,
		ContentSource:   sourceObject,
		FixitySource:    sourceObject,
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
	objRoot := tempObject(t, fixtureObj)
	// writable FS for tmpdir
	writeFS, err := local.NewFS(objRoot)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := ocflv1.GetObject(ctx, writeFS, fixtureObj.Path)
	if err != nil {
		t.Fatal(err)
	}
	alg := obj.Inventory.DigestAlgorithm
	objID := obj.Inventory.ID
	// update with invalid id
	t.Run("invalid-id", func(t *testing.T) {
		stage := &ocfl.Stage{DigestAlgorithm: alg}
		badID := "wrong"
		err := ocflv1.Commit(ctx, writeFS, obj.Path, badID, stage)
		if err == nil {
			t.Error("Commit() didn't return error for invalid object id")
		}
	})
	// committing into directory with existing files should fail
	t.Run("invalid-objpath-existing", func(t *testing.T) {
		stage := &ocfl.Stage{DigestAlgorithm: obj.Inventory.DigestAlgorithm}
		parentDir := path.Dir(obj.Path)
		err := ocflv1.Commit(ctx, writeFS, parentDir, objID, stage)
		if err == nil {
			t.Error("Commit() didn't return error for invalid object path")
		}
	})
	t.Run("invalid-objpath-isfile", func(t *testing.T) {
		stage := &ocfl.Stage{DigestAlgorithm: obj.Inventory.DigestAlgorithm}
		badPath := path.Join(obj.Path, "inventory.json")
		err := ocflv1.Commit(ctx, writeFS, badPath, objID, stage)
		if err == nil {
			t.Error("Commit() didn't return error for invalid object path")
		}
	})
	t.Run("invalid-head", func(t *testing.T) {
		stage := &ocfl.Stage{DigestAlgorithm: obj.Inventory.DigestAlgorithm}
		badHead := obj.Inventory.Head.Num()
		err := ocflv1.Commit(ctx, writeFS, obj.Path, objID, stage, ocflv1.WithHEAD(badHead))
		if err == nil {
			t.Error("Commit() didn't return error for invalid option WithHEAD value")
		}
	})
	t.Run("invalid-spec", func(t *testing.T) {
		stage := &ocfl.Stage{DigestAlgorithm: obj.Inventory.DigestAlgorithm}
		// test fixture use ocfl v1.1
		err := ocflv1.Commit(ctx, writeFS, obj.Path, objID, stage, ocflv1.WithOCFLSpec(ocfl.Spec1_0))
		if err == nil {
			t.Error("Commit() didn't return error for invalid option WithOCFLSpec value")
		}
	})
	t.Run("invalid-alg", func(t *testing.T) {
		commitAlg := `sha256`
		if alg == ocfl.SHA256 {
			commitAlg = ocfl.SHA512
		}
		// test fixture use ocfl v1.1
		stage := &ocfl.Stage{DigestAlgorithm: commitAlg}
		err := ocflv1.Commit(ctx, writeFS, obj.Path, objID, stage)
		if err == nil {
			t.Error("Commit() didn't return error for stage with different alg")
		}
	})
	t.Run("incomplete-stage", func(t *testing.T) {
		// stage state not provided by content
		stage := &ocfl.Stage{
			DigestAlgorithm: alg,
			State:           ocfl.DigestMap{"abc": []string{"file.txt"}},
		}
		err := ocflv1.Commit(ctx, writeFS, obj.Path, objID, stage)
		if err == nil {
			t.Fatal("Commit() didn't return error for incomplete stage")
		}
		var commitErr *ocflv1.CommitError
		if !errors.As(err, &commitErr) {
			t.Fatal("invalid Commit() didn't return CommitError")
		}
		if commitErr.Dirty {
			t.Error("Commit() with incomplete stage returned 'dirty' error")
		}
	})
	t.Run("update-1", func(t *testing.T) {
		stage, err := obj.Stage(0)
		if err != nil {
			t.Fatal(err)
		}
		testContent := map[string][]byte{
			"delete.txt":     []byte("This file will be deleted"),
			"rename-src.txt": []byte("This file will be renamed"),
			"updated.txt":    []byte("This file will be updated"),
			"unchanged.txt":  []byte("This file will be unchanged"),
		}
		updates, err := ocfl.StageBytes(testContent, alg, ocfl.MD5)
		if err != nil {
			t.Fatal(err)
		}
		if err := stage.Overlay(updates); err != nil {
			t.Fatal(err)
		}
		if err := ocflv1.Commit(ctx, writeFS, obj.Path, objID, stage); err != nil {
			t.Fatal(err)
		}
		// validite
		updatedObj, result := ocflv1.ValidateObject(ctx, writeFS, obj.Path)
		if err := result.Err(); err != nil {
			t.Fatal(err)
		}
		newInv := updatedObj.Inventory
		// check that new inventory has new content in fixity
		md5fixity := newInv.Fixity[ocfl.MD5]
		if len(md5fixity.Digests()) == 0 {
			t.Fatal("inventory should have md5 block in fixity")
		}
		// check that expected paths exist
		if !stage.State.Eq(newInv.Version(0).State) {
			t.Fatal("new version doesn't have expected state")
		}
		// md5 fixity should include entries for all new content
		md5paths := newInv.Fixity[ocfl.MD5].Paths()
		for name := range testContent {
			expectPath := path.Join(newInv.Head.String(), newInv.ContentDirectory, name)
			if !slices.Contains(md5paths, expectPath) {
				t.Fatalf("new inventory fixity doesn't include md5 doesn't include: %q", expectPath)
			}
		}

	})

	t.Run("update-2", func(t *testing.T) {
		if err := obj.ReadInventory(ctx); err != nil {
			t.Fatal(err)
		}
		stage, err := obj.Stage(0)
		if err != nil {
			t.Fatal(err)
		}
		testContent := map[string][]byte{
			"updated.txt": []byte("This file has been updated"),
		}
		updates, err := ocfl.StageBytes(testContent, alg, ocfl.MD5)
		if err != nil {
			t.Fatal(err)
		}
		if err := stage.Overlay(updates); err != nil {
			t.Fatal(err)
		}
		stage.State.Remap(
			ocfl.Rename("rename-src.txt", "rename-dst.txt"),
			ocfl.Remove("delete.txt"),
		)
		if err := ocflv1.Commit(ctx, writeFS, obj.Path, obj.Inventory.ID, stage); err != nil {
			t.Fatal(err)
		}
		updatedObj, result := ocflv1.ValidateObject(ctx, writeFS, obj.Path)
		if err := result.Err(); err != nil {
			t.Fatal(err)
		}
		newState := updatedObj.Inventory.Version(0).State
		if newState.GetDigest("delete.txt") != "" {
			t.Fatal("expected 'delete.txt' to be removed")
		}
		if newState.GetDigest("rename-src.txt") != "" {
			t.Fatal("expected 'rename-src.txt' to be removed")
		}
		if newState.GetDigest("rename-dst.txt") == "" {
			t.Fatal("expected 'rename-dst.txt' to exist")
		}
		// check updated path
		prevState := updatedObj.Inventory.Version(updatedObj.Inventory.Head.Num() - 1).State
		updatedPath := "updated.txt"
		prevDigest := prevState.GetDigest(updatedPath)
		newDigest := newState.GetDigest(updatedPath)
		if prevDigest == "" || prevDigest == newDigest {
			t.Fatalf("expected '%s' to have changed", updatedPath)
		}
	})

}

// creates a temporary directory and copies all files in the object into the
// directory, returning the tmp directory root. The object will be located at
// obj.Path in the new tmp directory. The caller should remember to removeall
// the temp directory.
func tempObject(t *testing.T, obj *ocfl.ObjectRoot) string {
	t.Helper()
	ctx := context.Background()
	tmpdir := t.TempDir()
	ocfl.Files(ctx, obj.FS, obj.Path)(func(file ocfl.FileInfo, _ error) bool {
		name := file.Path
		dir := path.Dir(name)
		if err := os.MkdirAll(filepath.Join(tmpdir, filepath.FromSlash(dir)), 0777); err != nil {
			t.Fatal(err)
			return false
		}
		f, err := obj.FS.OpenFile(ctx, name)
		if err != nil {
			t.Fatal(err)
			return false
		}
		defer f.Close()
		w, err := os.Create(filepath.Join(tmpdir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
			return false
		}
		if _, err := io.Copy(w, f); err != nil {
			t.Fatal(err)
			return false
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
			return false
		}
		return true
	})
	return tmpdir
}
