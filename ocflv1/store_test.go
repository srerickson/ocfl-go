package ocflv1_test

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/memfs"
	"github.com/srerickson/ocfl-go/extension"
	"github.com/srerickson/ocfl-go/internal/testutil"
	"github.com/srerickson/ocfl-go/ocflv1"
)

var storePath = filepath.Join(`..`, `testdata`, `store-fixtures`, `1.0`)

type storeTest struct {
	name   string
	size   int
	layout extension.Layout
}

var testStoreLayout = testutil.CustomLayout{Prefix: ""}

func TestGetStore(t *testing.T) {
	ctx := context.Background()
	t.Run("not a storage root", func(t *testing.T) {
		if _, err := ocflv1.GetStore(ctx, ocfl.DirFS("."), "."); err == nil {
			t.Fatal("expected an error")
		}
	})
	// map to store to expected # of objects
	var storeTests = []storeTest{
		{name: `good-stores/reg-extension-dir-root`, size: 1, layout: nil},
		{name: `good-stores/unreg-extension-dir-root`, size: 1, layout: testStoreLayout},
		{name: `good-stores/simple-root`, size: 3, layout: testStoreLayout},
	}
	for _, sttest := range storeTests {
		t.Run(sttest.name, func(t *testing.T) {
			var fsys ocfl.FS
			var root string
			if strings.HasSuffix(sttest.name, `.zip`) {
				root = "."
				zreader, err := zip.OpenReader(filepath.Join(storePath, sttest.name))
				if err != nil {
					t.Fatal(err)
				}
				defer zreader.Close()
				fsys = ocfl.NewFS(zreader)
			} else {
				fsys = ocfl.NewFS(os.DirFS(storePath))
				root = sttest.name
			}
			store, err := ocflv1.GetStore(ctx, fsys, root)
			if err != nil {
				t.Fatal(err)
			}
			if store.LayoutName() == "" {
				// set custom layout defined in test
				store.Layout = sttest.layout
			} else {
				// read extension from store's layout config
				err := store.ReadLayout(ctx)
				if err != nil {
					t.Fatal(err)
				}
			}
			if store.Layout == nil {
				t.Fatal("store should have set layout")
			}
			scanFn := func(obj *ocflv1.Object, err error) error {
				return err
			}

			if err := store.Objects(ctx, scanFn); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStoreEachObject(t *testing.T) {
	ctx := context.Background()
	// map to store to expected # of objects
	var storeTests = []storeTest{
		{name: `good-stores/reg-extension-dir-root`, size: 1, layout: nil},
		{name: `good-stores/unreg-extension-dir-root`, size: 1, layout: testStoreLayout},
		{name: `good-stores/simple-root`, size: 3, layout: testStoreLayout},
		{name: `warn-stores/fedora-root.zip`, size: 176, layout: testStoreLayout},
		{name: `bad-stores/E072_root_with_file_not_in_object`, size: 1, layout: testStoreLayout},
		{name: `bad-stores/E073_root_with_empty_dir.zip`, size: 0, layout: testStoreLayout},
	}
	for _, sttest := range storeTests {
		t.Run(sttest.name, func(t *testing.T) {
			var fsys ocfl.FS
			var root string
			var store *ocflv1.Store
			var err error
			if strings.HasSuffix(sttest.name, `.zip`) {
				root = "."
				zreader, err := zip.OpenReader(filepath.Join(storePath, sttest.name))
				if err != nil {
					t.Fatal(err)
				}
				defer zreader.Close()
				fsys = ocfl.NewFS(zreader)
			} else {
				fsys = ocfl.NewFS(os.DirFS(storePath))
				root = sttest.name
			}
			store, err = ocflv1.GetStore(ctx, fsys, root)
			if err != nil {
				t.Fatal(err)
			}
			numObjs := 0
			scanFn := func(obj *ocflv1.Object, err error) error {
				if err != nil {
					return err
				}
				numObjs++
				return nil
			}
			if scanErr := store.Objects(ctx, scanFn); scanErr != nil {
				t.Fatal(scanErr)
			}
			if numObjs != sttest.size {
				t.Fatalf("expected %d objects, got %d", sttest.size, numObjs)
			}
		})
	}
}

func TestStoreCommit(t *testing.T) {
	storePath := "storage-root"
	ctx := context.Background()
	storeFS := memfs.New() // store
	stageContent := fstest.MapFS{
		`stage1/tmp.txt`:       &fstest.MapFile{Data: []byte(`content1`)},
		`stage3/a/tmp.txt`:     &fstest.MapFile{Data: []byte(`content2`)},
		`stage3/a/another.txt`: &fstest.MapFile{Data: []byte(`content3`)},
	}
	// stage1 commit is from storeFS, copy files
	for n := range stageContent {
		if !strings.HasPrefix(n, "stage1") {
			continue
		}
		f, err := stageContent.Open(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := storeFS.Write(ctx, n, f); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	// initialize store
	if err := ocflv1.InitStore(ctx, storeFS, storePath, &ocflv1.InitStoreConf{
		Spec: ocfl.Spec{1, 0},
	}); err != nil {
		t.Fatal(err)
	}
	store, err := ocflv1.GetStore(ctx, storeFS, storePath)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("without options", func(t *testing.T) {
		stage := ocfl.NewStage(ocfl.SHA256)
		if err = store.Commit(ctx, "object-0", stage); err != nil {
			t.Fatal(err)
		}
		obj, err := store.GetObject(ctx, "object-0")
		if err != nil {
			t.Fatal(err)
		}
		if err := obj.Validate(ctx).Err(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("with invalid spec", func(t *testing.T) {
		stage := ocfl.NewStage(ocfl.SHA256) // empty stage
		err := store.Commit(ctx, "object-0", stage, ocflv1.WithOCFLSpec(ocfl.Spec{1, 1}))
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("add file", func(t *testing.T) {
		stage := ocfl.NewStage(ocfl.SHA256)
		if err := stage.AddFS(ctx, storeFS, "stage1"); err != nil {
			t.Fatal(err)
		}
		if stage.State.GetDigest("tmp.txt") == "" {
			t.Fatal("missing expected digest")
		}
		if err = store.Commit(ctx, "object-1", stage,
			ocflv1.WithContentDir("foo"),
			ocflv1.WithVersionPadding(2),
			ocflv1.WithUser(ocfl.User{Name: "Will", Address: "mailto:Will@email.com"}),
			ocflv1.WithMessage("first commit"),
		); err != nil {
			t.Fatal(err)
		}
	})

	// stage 2 - remove "tmp.txt"
	t.Run("remove file", func(t *testing.T) {
		obj, err := store.GetObject(ctx, "object-1")
		if err != nil {
			t.Fatal()
		}
		state, err := obj.State(0)
		if err != nil {
			t.Fatal(err)
		}
		stage := ocfl.NewStage(state.Alg)
		stage.State, err = state.DigestMap.Remap(ocfl.Remove("tmp.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Commit(ctx, "object-1", stage,
			ocflv1.WithUser(ocfl.User{Name: "Wanda", Address: "mailto:wanda@email.com"}),
			ocflv1.WithMessage("second commit")); err != nil {
			t.Fatal(err)
		}
	})

	// v3 - new files and rename one
	t.Run("add and rename files", func(t *testing.T) {
		_, err = store.GetObject(ctx, "object-1")
		if err != nil {
			t.Fatal(err)
		}
		stage := ocfl.NewStage(ocfl.SHA256)
		if err = stage.AddFS(ctx, ocfl.NewFS(stageContent), "stage3"); err != nil {
			t.Fatal(err)
		}
		// rename one of the staged files
		stage.State, err = stage.State.Remap(ocfl.Rename("a/tmp.txt", "tmp.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Commit(ctx, "object-1", stage,
			ocflv1.WithUser(ocfl.User{Name: "Woody", Address: "mailto:Woody@email.com"}),
			ocflv1.WithMessage("third commit"),
		); err != nil {
			t.Fatal(err)
		}
	})

	// v4 - update one of the files by writing
	t.Run("change file", func(t *testing.T) {
		obj, err := store.GetObject(ctx, "object-1")
		if err != nil {
			t.Fatal(err)
		}
		objState, err := obj.State(0)
		if err != nil {
			t.Fatal(err)
		}
		stage4fsys := memfs.New()
		if _, err := stage4fsys.Write(ctx, "a/another.txt", strings.NewReader("fresh deats")); err != nil {
			t.Fatal(err)
		}
		stage := ocfl.NewStage(objState.Alg)
		stage.State = objState.DigestMap
		if err := stage.AddFS(ctx, stage4fsys, "."); err != nil {
			t.Fatal(err)
		}
		if err := store.Commit(ctx, "object-1", stage,
			ocflv1.WithUser(ocfl.User{Name: "Winnie", Address: "mailto:Winnie@no.com"}),
			ocflv1.WithMessage("last commit"),
		); err != nil {
			t.Fatal(err)
		}
	})

	// validate store
	if err := store.Validate(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	// validate object
	obj, err := store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal(err)
	}
	if result := obj.Validate(ctx); result.Err() != nil {
		t.Fatal("object is invalid", result.Err())
	}
	inv := obj.Inventory
	if inv.ContentDirectory != "foo" {
		t.Fatal("expected foo")
	}
	if inv.DigestAlgorithm != ocfl.SHA256 {
		t.Fatalf("expected sha256")
	}
	if inv.Head.Padding() != 2 {
		t.Fatalf("expected 2")
	}
	if inv.Head.Num() != 4 {
		t.Fatal("expected 4")
	}
	for num, ver := range inv.Versions {
		if ver.User == nil || ver.User.Name == "" {
			t.Fatal("expected a user for version", num)
		}
		if ver.Message == "" {
			t.Fatal("expected a message for version", num)
		}
	}
	finalState, err := obj.State(0)
	if err != nil {
		t.Fatal(err)
	}
	if finalState.GetDigest("tmp.txt") == "" {
		t.Fatal("missing expected file")
	}
	if finalState.GetDigest("a/another.txt") == "" {
		t.Fatal("missing expected file")
	}
	if finalState.NumPaths() != 2 {
		t.Fatal("expected only two items")
	}
}
