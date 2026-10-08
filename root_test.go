package ocfl_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/extension"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/local"
)

func TestNewRoot(t *testing.T) {
	ctx := context.Background()
	t.Run("fixture reg-extension-dir-root", func(t *testing.T) {
		fsys := ocflfs.DirFS(storeFixturePath)
		dir := `1.0/good-stores/reg-extension-dir-root`
		root, err := ocfl.NewRoot(ctx, fsys, dir)
		be.NilErr(t, err)
		be.Equal(t, ocfl.Spec1_0, root.Spec())
		obj, err := root.NewObject(ctx, "ark:123/abc")
		be.NilErr(t, err)
		be.True(t, obj.Exists())
	})
	t.Run("fixture simple-root", func(t *testing.T) {
		fsys := ocflfs.DirFS(storeFixturePath)
		dir := `1.0/good-stores/simple-root`
		root, err := ocfl.NewRoot(ctx, fsys, dir)
		be.NilErr(t, err)
		be.Equal(t, ocfl.Spec1_0, root.Spec())
	})

}

func TestRoot_Example(t *testing.T) {
	ctx := context.Background()
	fsys, err := local.NewFS(t.TempDir())
	be.NilErr(t, err)
	// new root settings
	dir := `new-root`
	desc := "a new root"
	layout := extension.Ext0004()
	newRoot, err := ocfl.NewRoot(ctx, fsys, dir, ocfl.InitRoot(ocfl.Spec1_1, desc, layout))
	be.NilErr(t, err)
	be.Equal(t, layout.Name(), newRoot.Layout().Name())
	be.Equal(t, ocfl.Spec1_1, newRoot.Spec())
	be.Equal(t, desc, newRoot.Description())
	// create an object
	objID := "object-1"
	stage, err := newRoot.NewStage(ctx, objID, ocfl.StageWithDigestAlgorithm(digest.SHA256))
	be.NilErr(t, err)
	be.Equal(t, stage.Update().ID(), objID)
	be.NilErr(t, stage.AddBytes("file.txt", []byte("readme readme readme")))
	be.NilErr(t, stage.Finalize("first version", ocfl.User{Name: "Stinky & Dirty"}))
	obj, err := newRoot.Apply(ctx, stage.Update(), stage.Content())
	be.NilErr(t, err)
	be.Equal(t, newRoot, obj.Root())
	// re-open and validate object
	sameRoot, err := ocfl.NewRoot(ctx, fsys, dir)
	be.NilErr(t, err)
	be.Equal(t, layout.Name(), sameRoot.Layout().Name())
	be.Equal(t, ocfl.Spec1_1, sameRoot.Spec())
	be.Equal(t, desc, sameRoot.Description())
	sameObj, err := sameRoot.NewObject(ctx, objID)
	be.NilErr(t, err)
	be.NilErr(t, ocfl.ValidateObject(ctx, obj.FS(), obj.Path()).Err())
	be.Equal(t, objID, sameObj.ID())
}

func TestRoot_ObjectsBatch(t *testing.T) {
	ctx := context.Background()
	fsys := ocflfs.DirFS(filepath.Join(`testdata`, `store-fixtures`))

	t.Run("simple-root", func(t *testing.T) {
		dir := `1.0/good-stores/simple-root`
		root, err := ocfl.NewRoot(ctx, fsys, dir)
		be.NilErr(t, err)
		count := 0
		for obj, err := range root.ObjectsBatch(ctx, 2) {
			be.NilErr(t, err)
			count++
			be.True(t, obj.Exists())
			be.Equal(t, root, obj.Root())
		}
		be.Equal(t, 3, count)
	})

	t.Run("break iteration", func(t *testing.T) {
		dir := `1.0/good-stores/simple-root`
		root, err := ocfl.NewRoot(ctx, fsys, dir)
		be.NilErr(t, err)
		// check that iterator doesn't after break: this will panic
		defer func() {
			if err := recover(); err != nil {
				t.Fatal(err)
			}
		}()
		for range root.ObjectsBatch(ctx, 1) {
			break
		}
	})

	t.Run("root with error", func(t *testing.T) {
		dir := `1.0/bad-stores/multi_level_errors`
		root, err := ocfl.NewRoot(ctx, fsys, dir)
		be.NilErr(t, err)
		count := 0
		// iterate over all declarations, even if there are errors
		for range root.ObjectsBatch(ctx, 1) {
			count++
		}
		be.Equal(t, 3, count)
	})

}

func TestRoot_ObjectDeclarations(t *testing.T) {
	ctx := context.Background()
	fsys := ocflfs.DirFS(filepath.Join(`testdata`, `store-fixtures`))
	dir := `1.0/good-stores/simple-root`
	root, err := ocfl.NewRoot(ctx, fsys, dir)
	be.NilErr(t, err)

	t.Run("simple-root", func(t *testing.T) {
		count := 0
		for ref, err := range root.ObjectDeclarations(ctx) {
			be.NilErr(t, err)
			be.Nonzero(t, ref.Info)
			count++
		}
		be.Equal(t, 3, count)
	})

	t.Run("break iteration", func(t *testing.T) {
		// check that iterator doesn't after break: this will panic
		defer func() {
			if err := recover(); err != nil {
				t.Fatal(err)
			}
		}()
		for range root.ObjectDeclarations(ctx) {
			break
		}
	})
}

func TestRoot_ValidateObject(t *testing.T) {
	ctx := context.Background()
	fsys := ocflfs.DirFS(filepath.Join(`testdata`, `store-fixtures`))
	dir := `1.0/good-stores/simple-root`
	root, err := ocfl.NewRoot(ctx, fsys, dir)
	be.NilErr(t, err)
	t.Run("simple", func(t *testing.T) {
		objPath := "http%3A%2F%2Fexample.org%2Fminimal_mixed_digests"
		valid := root.ValidateObjectDir(ctx, objPath)
		be.NilErr(t, valid.Err())
	})
	t.Run("not exist", func(t *testing.T) {
		objPath := "none"
		err = root.ValidateObjectDir(ctx, objPath).Err()
		be.True(t, err != nil)
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})
}

func TestNewRoot_InvalidConfig(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		fsMap          fstest.MapFS
		expectedErrMsg string
	}{
		{
			name: "unknown extension",
			fsMap: fstest.MapFS{
				"root/0=ocfl_1.0": &fstest.MapFile{
					Data: []byte("ocfl_1.0\n"),
				},
				"root/ocfl_layout.json": &fstest.MapFile{
					Data: []byte(`{"extension": "9999-unknown-extension"}`),
				},
				"root/extensions/9999-unknown-extension/config.json": &fstest.MapFile{
					Data: []byte(`{"extensionName": "9999-unknown-extension"}`),
				},
			},
			expectedErrMsg: `unrecognized extension name: "9999-unknown-extension"`,
		},
		{
			name: "missing extensionName",
			fsMap: fstest.MapFS{
				"root/0=ocfl_1.0": &fstest.MapFile{
					Data: []byte("ocfl_1.0\n"),
				},
				"root/ocfl_layout.json": &fstest.MapFile{
					Data: []byte(`{"extension": "0004-hashed-n-tuple-storage-layout", "description": "test layout"}`),
				},
				"root/extensions/0004-hashed-n-tuple-storage-layout/config.json": &fstest.MapFile{
					Data: []byte(`{"digestAlgorithm": "sha256", "tupleSize": 2, "numberOfTuples": 3}`),
				},
			},
			expectedErrMsg: `missing required field in extension config: "extensionName"`,
		},
		{
			name: "empty extensionName",
			fsMap: fstest.MapFS{
				"root/0=ocfl_1.0": &fstest.MapFile{
					Data: []byte("ocfl_1.0\n"),
				},
				"root/ocfl_layout.json": &fstest.MapFile{
					Data: []byte(`{"extension": "0004-hashed-n-tuple-storage-layout"}`),
				},
				"root/extensions/0004-hashed-n-tuple-storage-layout/config.json": &fstest.MapFile{
					Data: []byte(`{"extensionName": "", "digestAlgorithm": "sha256", "tupleSize": 2, "numberOfTuples": 3}`),
				},
			},
			expectedErrMsg: `missing required field in extension config: "extensionName"`,
		},
		{
			name: "invalid JSON in layout config",
			fsMap: fstest.MapFS{
				"root/0=ocfl_1.0": &fstest.MapFile{
					Data: []byte("ocfl_1.0\n"),
				},
				"root/ocfl_layout.json": &fstest.MapFile{
					Data: []byte(`{invalid json}`),
				},
			},
			expectedErrMsg: "storage root layout config is invalid: in root/ocfl_layout.json:",
		},
		{
			name: "invalid JSON in extension config",
			fsMap: fstest.MapFS{
				"root/0=ocfl_1.0": &fstest.MapFile{
					Data: []byte("ocfl_1.0\n"),
				},
				"root/ocfl_layout.json": &fstest.MapFile{
					Data: []byte(`{"extension": "0004-hashed-n-tuple-storage-layout", "description": "test layout"}`),
				},
				"root/extensions/0004-hashed-n-tuple-storage-layout/config.json": &fstest.MapFile{
					Data: []byte(`{invalid json}`),
				},
			},
			expectedErrMsg: "extension config has errors: in root/extensions/0004-hashed-n-tuple-storage-layout/config.json",
		},
		{
			name: "invalid extension config",
			fsMap: fstest.MapFS{
				"root/0=ocfl_1.0": &fstest.MapFile{
					Data: []byte("ocfl_1.0\n"),
				},
				"root/ocfl_layout.json": &fstest.MapFile{
					Data: []byte(`{"extension": "0006-flat-omit-prefix-storage-layout", "description": "test layout"}`),
				},
			},
			// the default config is used if there is no extension config file,
			// however the default config for
			// 0006-flat-omit-prefix-storage-layout is invalid.
			expectedErrMsg: "required field not set in extension config",
		},
		{
			name: "extension is not a layout",
			fsMap: fstest.MapFS{
				"root/0=ocfl_1.0": &fstest.MapFile{
					Data: []byte("ocfl_1.0\n"),
				},
				"root/ocfl_layout.json": &fstest.MapFile{
					Data: []byte(`{"extension": "0009-digest-algorithms", "description": "test layout"}`),
				},
			},
			expectedErrMsg: "extension is not a layout as expected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := ocflfs.NewWrapFS(tt.fsMap)
			_, err := ocfl.NewRoot(ctx, fsys, "root")
			be.True(t, err != nil)
			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Error("error message doesn not include expected text")
				t.Logf("--expected: %q", tt.expectedErrMsg)
				t.Logf("--got     : %q", err.Error())
			}
		})
	}
}

// Objects created through a Root must not declare a newer OCFL spec than the
// root (E081): new objects default to the root's spec, and explicit specs newer
// than the root's are rejected.
func TestRoot_NewUpdate_Spec(t *testing.T) {
	ctx := context.Background()
	user := ocfl.User{Name: "n"}
	newRoot := func(t *testing.T, spec ocfl.Spec) (*ocfl.Root, ocflfs.FS) {
		fsys, err := local.NewFS(t.TempDir())
		be.NilErr(t, err)
		root, err := ocfl.NewRoot(ctx, fsys, ".", ocfl.InitRoot(spec, "", extension.Ext0004()))
		be.NilErr(t, err)
		be.Equal(t, spec, root.Spec())
		return root, fsys
	}
	// update creates a version of "obj1" in root with one file
	update := func(t *testing.T, root *ocfl.Root, name string, opts ...ocfl.UpdateOption) (*ocfl.Object, error) {
		t.Helper()
		stage, err := root.NewStage(ctx, "obj1")
		be.NilErr(t, err)
		be.NilErr(t, stage.AddBytes(name, []byte(name)))
		if err := stage.Finalize("msg", user, opts...); err != nil {
			return nil, err
		}
		return root.Apply(ctx, stage.Update(), stage.Content())
	}
	t.Run("new object in 1.0 root defaults to 1.0", func(t *testing.T) {
		root, _ := newRoot(t, ocfl.Spec1_0)
		obj, err := update(t, root, "a.txt")
		be.NilErr(t, err)
		be.Equal(t, ocfl.Spec1_0, obj.Spec())
		be.NilErr(t, root.ValidateObject(ctx, "obj1").Err())
		// re-open the object and check the declaration on disk
		reopened, err := root.NewObject(ctx, "obj1", ocfl.ObjectMustExist())
		be.NilErr(t, err)
		be.Equal(t, ocfl.Spec1_0, reopened.Spec())
	})
	t.Run("new object in 1.1 root defaults to 1.1", func(t *testing.T) {
		root, _ := newRoot(t, ocfl.Spec1_1)
		obj, err := update(t, root, "a.txt")
		be.NilErr(t, err)
		be.Equal(t, ocfl.Spec1_1, obj.Spec())
		be.NilErr(t, root.ValidateObject(ctx, "obj1").Err())
	})
	t.Run("explicit 1.1 in 1.0 root is rejected", func(t *testing.T) {
		root, fsys := newRoot(t, ocfl.Spec1_0)
		_, err := update(t, root, "a.txt", ocfl.UpdateWithOCFLSpec(ocfl.Spec1_1))
		be.True(t, errors.Is(err, ocfl.ErrObjectSpecExceedsRoot))
		be.In(t, "1.1", err.Error())
		be.In(t, "1.0", err.Error())
		// nothing written to the object directory
		objPath, err := root.ResolveID("obj1")
		be.NilErr(t, err)
		_, err = ocflfs.ReadDir(ctx, fsys, objPath)
		be.True(t, errors.Is(err, fs.ErrNotExist))
	})
	t.Run("the root's spec is checked by objects from the root", func(t *testing.T) {
		root, _ := newRoot(t, ocfl.Spec1_0)
		obj, err := root.NewObject(ctx, "obj1")
		be.NilErr(t, err)
		stage := obj.NewStage()
		be.NilErr(t, stage.AddBytes("a.txt", []byte("a")))
		err = stage.Finalize("msg", user, ocfl.UpdateWithOCFLSpec(ocfl.Spec1_1))
		be.True(t, errors.Is(err, ocfl.ErrObjectSpecExceedsRoot))
	})
	t.Run("upgrading existing object above 1.0 root is rejected", func(t *testing.T) {
		root, _ := newRoot(t, ocfl.Spec1_0)
		_, err := update(t, root, "a.txt")
		be.NilErr(t, err)
		_, err = update(t, root, "b.txt", ocfl.UpdateWithOCFLSpec(ocfl.Spec1_1))
		be.True(t, errors.Is(err, ocfl.ErrObjectSpecExceedsRoot))
		// a plain update (no spec option) keeps working at 1.0
		obj, err := update(t, root, "b.txt")
		be.NilErr(t, err)
		be.Equal(t, ocfl.V(2), obj.Head())
		be.Equal(t, ocfl.Spec1_0, obj.Spec())
	})
	t.Run("explicit 1.0 in 1.1 root is allowed", func(t *testing.T) {
		root, _ := newRoot(t, ocfl.Spec1_1)
		obj, err := update(t, root, "a.txt", ocfl.UpdateWithOCFLSpec(ocfl.Spec1_0))
		be.NilErr(t, err)
		be.Equal(t, ocfl.Spec1_0, obj.Spec())
		be.NilErr(t, root.ValidateObject(ctx, "obj1").Err())
	})
	t.Run("the root's spec is saved with the update", func(t *testing.T) {
		root, _ := newRoot(t, ocfl.Spec1_0)
		upd, err := root.NewUpdate(ctx, "obj1")
		be.NilErr(t, err)
		saved, err := json.Marshal(upd)
		be.NilErr(t, err)
		var loaded ocfl.ObjectUpdate
		be.NilErr(t, json.Unmarshal(saved, &loaded))
		err = loaded.Finalize("msg", user, ocfl.UpdateWithOCFLSpec(ocfl.Spec1_1))
		be.True(t, errors.Is(err, ocfl.ErrObjectSpecExceedsRoot))
	})
	t.Run("object without root defaults to latest spec", func(t *testing.T) {
		fsys, err := local.NewFS(t.TempDir())
		be.NilErr(t, err)
		upd, err := ocfl.NewUpdate(ctx, fsys, "obj1", "obj1")
		be.NilErr(t, err)
		be.NilErr(t, upd.Finalize("msg", user))
		obj, err := upd.Apply(ctx, fsys, "obj1", nil)
		be.NilErr(t, err)
		be.Equal(t, ocfl.Spec1_1, obj.Spec())
	})
}

// Root.ValidateObject reports E081 for objects declaring a spec newer than the
// root's; plain ValidateObject (no root) does not.
func TestRoot_ValidateObject_E081(t *testing.T) {
	ctx := context.Background()
	fsys, err := local.NewFS(t.TempDir())
	be.NilErr(t, err)
	root, err := ocfl.NewRoot(ctx, fsys, ".", ocfl.InitRoot(ocfl.Spec1_0, "", extension.Ext0004()))
	be.NilErr(t, err)
	objPath, err := root.ResolveID("obj1")
	be.NilErr(t, err)
	// create a 1.1 object inside the 1.0 root, bypassing the root
	stage, err := ocfl.NewStage(ctx, fsys, objPath, "obj1")
	be.NilErr(t, err)
	upd := stage.Update()
	be.NilErr(t, stage.AddBytes("a.txt", []byte("hi")))
	be.NilErr(t, stage.Finalize("msg", ocfl.User{Name: "n"}))
	obj, err := upd.Apply(ctx, fsys, objPath, stage.Content())
	be.NilErr(t, err)
	be.Equal(t, ocfl.Spec1_1, obj.Spec())

	// the object is valid on its own
	be.NilErr(t, ocfl.ValidateObject(ctx, fsys, objPath).Err())
	// but not as part of the 1.0 root
	for _, v := range []*ocfl.ObjectValidation{
		root.ValidateObject(ctx, "obj1"),
		root.ValidateObjectDir(ctx, objPath),
	} {
		err := v.Err()
		be.Nonzero(t, err)
		var vErr *ocfl.ValidationError
		be.True(t, errors.As(err, &vErr))
		be.Equal(t, "E081", vErr.Code)
		be.Equal(t, "1.0", vErr.Spec)
	}
	// the root-created object is fine, though
	stage2, err := root.NewStage(ctx, "obj2")
	be.NilErr(t, err)
	be.NilErr(t, stage2.AddBytes("a.txt", []byte("hi")))
	be.NilErr(t, stage2.Finalize("msg", ocfl.User{Name: "n"}))
	_, err = root.Apply(ctx, stage2.Update(), stage2.Content())
	be.NilErr(t, err)
	be.NilErr(t, root.ValidateObject(ctx, "obj2").Err())
}
