package ocflv1_test

import (
	"archive/zip"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/extensions"
	"github.com/srerickson/ocfl/ocflv1"
)

var storePath = filepath.Join(`..`, `testdata`, `store-fixtures`, `1.0`)

type storeTest struct {
	name   string
	size   int
	layout extensions.Layout
}

// layout used for fixture stores w/o layout file
var testStoreLayout customLayout

type customLayout struct{}

var _ extensions.Layout = (*customLayout)(nil)

func (l customLayout) Name() string {
	return "uri-encode"
}

func (l customLayout) NewFunc() (extensions.LayoutFunc, error) {
	return func(id string) (string, error) {
		return url.QueryEscape(id), nil
	}, nil
}

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
				err := store.SetLayout(sttest.layout)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				// read extension from store's layout config
				err := store.ReadLayout(ctx)
				if err != nil {
					t.Fatal(err)
				}
			}
			if !store.LayoutOK() {
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
