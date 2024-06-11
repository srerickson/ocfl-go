package ocflv1_test

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

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
			store.Objects(ctx)(func(obj *ocflv1.Object, err error) bool {
				if err != nil {
					t.Error(err)
				}
				return true
			})
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
			store.Objects(ctx)(func(obj *ocflv1.Object, err error) bool {
				if err != nil {
					t.Error(err)
					return false
				}
				numObjs++
				return true
			})
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
	// initialize store
	if err := ocflv1.InitStore(ctx, storeFS, storePath, &ocflv1.InitStoreConf{
		Spec: ocfl.Spec1_0,
	}); err != nil {
		t.Fatal(err)
	}
	store, err := ocflv1.GetStore(ctx, storeFS, storePath)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("without options", func(t *testing.T) {
		stage := &ocfl.Stage{State: ocfl.DigestMap{}, DigestAlgorithm: "sha256"}
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
		stage := &ocfl.Stage{State: ocfl.DigestMap{}, DigestAlgorithm: "sha256"}
		err := store.Commit(ctx, "object-0", stage, ocflv1.WithOCFLSpec(ocfl.Spec1_1))
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("add file", func(t *testing.T) {
		newContent, err := ocfl.StageBytes(map[string][]byte{
			"file1.txt": []byte("content1"),
			"file2.txt": []byte("content2"),
		}, ocfl.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		if err = store.Commit(ctx, "object-1", newContent,
			ocflv1.WithContentDir("foo"),
			ocflv1.WithVersionPadding(2),
			ocflv1.WithUser(ocfl.User{Name: "Will", Address: "mailto:Will@email.com"}),
			ocflv1.WithMessage("first commit"),
		); err != nil {
			t.Fatal(err)
		}
		obj, err := store.GetObject(ctx, "object-1")
		if err != nil {
			t.Fatal(err)
		}
		expected := []string{"file1.txt", "file2.txt"}
		got := obj.Inventory.Version(0).State.Paths()
		sort.Strings(got)
		if !reflect.DeepEqual(expected, got) {
			t.Fatalf("expected=%v, got=%v", expected, got)
		}
	})

	// stage 2 - remove "file1.txt"
	t.Run("remove file", func(t *testing.T) {
		obj, err := store.GetObject(ctx, "object-1")
		if err != nil {
			t.Fatal()
		}
		stage, err := obj.Stage(0)
		if err != nil {
			t.Fatal(err)
		}
		stage.State.Remap(ocfl.Remove("file1.txt"))
		if err := store.Commit(ctx, "object-1", stage,
			ocflv1.WithUser(ocfl.User{Name: "Wanda", Address: "mailto:wanda@email.com"}),
			ocflv1.WithMessage("second commit")); err != nil {
			t.Fatal(err)
		}
		if err := obj.ReadInventory(ctx); err != nil {
			t.Fatal(err)
		}
		expected := []string{"file2.txt"}
		got := obj.Inventory.Version(0).State.Paths()
		sort.Strings(got)
		if !reflect.DeepEqual(expected, got) {
			t.Fatalf("expected=%v, got=%v", expected, got)
		}
	})

	// v3 - new files and rename one
	t.Run("add and rename files", func(t *testing.T) {
		newContent, err := ocfl.StageBytes(map[string][]byte{
			"file3.txt": []byte("content3"),
		}, ocfl.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		obj, err := store.GetObject(ctx, "object-1")
		if err != nil {
			t.Fatal(err)
		}
		stage, err := obj.Stage(0)
		if err != nil {
			t.Fatal(err)
		}
		if err := stage.Overlay(newContent); err != nil {
			t.Fatal(err)
		}
		stage.State.Remap(ocfl.Rename("file2.txt", "dir/file2.txt"))
		if err := store.Commit(ctx, "object-1", stage,
			ocflv1.WithUser(ocfl.User{Name: "Woody", Address: "mailto:Woody@email.com"}),
			ocflv1.WithMessage("third commit"),
		); err != nil {
			t.Fatal(err)
		}
		if err := obj.ReadInventory(ctx); err != nil {
			t.Fatal(err)
		}
		expected := []string{"dir/file2.txt", "file3.txt"}
		got := obj.Inventory.Version(0).State.Paths()
		sort.Strings(got)
		if !reflect.DeepEqual(expected, got) {
			t.Fatalf("expected=%v, got=%v", expected, got)
		}
	})

	// v4 - update one file and remove another
	t.Run("change file", func(t *testing.T) {
		newContent, err := ocfl.StageBytes(map[string][]byte{
			"file3.txt": []byte("changed"),
		}, ocfl.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		obj, err := store.GetObject(ctx, "object-1")
		if err != nil {
			t.Fatal(err)
		}
		stage, err := obj.Stage(0)
		if err != nil {
			t.Fatal(err)
		}
		if err := stage.Overlay(newContent); err != nil {
			t.Fatal(err)
		}
		stage.State.Remap(ocfl.Remove("dir/file2.txt"))
		if err := store.Commit(ctx, "object-1", stage,
			ocflv1.WithUser(ocfl.User{Name: "Winnie", Address: "mailto:Winnie@no.com"}),
			ocflv1.WithMessage("last commit"),
		); err != nil {
			t.Fatal(err)
		}
		if err := obj.ReadInventory(ctx); err != nil {
			t.Fatal(err)
		}
		expected := []string{"file3.txt"}
		got := obj.Inventory.Version(0).State.Paths()
		sort.Strings(got)
		if !reflect.DeepEqual(expected, got) {
			t.Fatalf("expected=%v, got=%v", expected, got)
		}
		// check file3
		digester := ocfl.NewDigester(ocfl.SHA256)
		digester.Write([]byte("changed"))
		expectDigest := digester.String()
		if len(obj.Inventory.Manifest[expectDigest]) < 1 {
			t.Fatal("object manifest doesn't include the expected digest")
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
	finalState := obj.Inventory.Version(0).State
	if finalState.NumPaths() != 1 {
		t.Fatal("expected only one item")
	}
}
