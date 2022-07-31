package ocflv1_test

import (
	"archive/zip"
	"context"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srerickson/ocfl/extensions"
	"github.com/srerickson/ocfl/ocflv1"
)

var storePath = filepath.Join(`testdata`, `store-fixtures`, `1.0`)

type storeTest struct {
	name   string
	size   int
	layout extensions.Layout
}

func TestGetStore(t *testing.T) {
	ctx := context.Background()
	// map to store to expected # of objects
	var storeTests = []storeTest{
		{name: `good-stores/reg-extension-dir-root`, size: 1, layout: nil},
		{name: `good-stores/unreg-extension-dir-root`, size: 1, layout: testStoreLayout},
		{name: `good-stores/simple-root`, size: 3, layout: testStoreLayout},
	}
	for _, sttest := range storeTests {
		t.Run(sttest.name, func(t *testing.T) {
			var fsys fs.FS
			var root string
			if strings.HasSuffix(sttest.name, `.zip`) {
				root = "."
				zreader, err := zip.OpenReader(filepath.Join(storePath, sttest.name))
				if err != nil {
					t.Fatal(err)
				}
				defer zreader.Close()
				fsys = zreader
			} else {
				fsys = os.DirFS(storePath)
				root = sttest.name
			}
			store, err := ocflv1.GetStore(ctx, fsys, root)
			if err != nil {
				t.Fatal(err)
			}
			if store.Config == nil {
				// set custom layout defined in test
				err := store.SetLayout(sttest.layout)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				// read extension from store's layout config
				err := store.ReadLayout(ctx, store.Config.Extension())
				if err != nil {
					t.Fatal(err)
				}
			}
			scan, err := store.ScanObjects(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			for p := range scan {
				obj, err := store.GetPath(ctx, p)
				if err != nil {
					t.Fatal(err)
				}
				inv, err := obj.Inventory(ctx)
				if err != nil {
					t.Fatal(err)
				}
				// test layout works
				_, err = store.GetID(ctx, inv.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestScanObjects(t *testing.T) {
	ctx := context.Background()

	// map to store to expected # of objects
	var storeTests = []storeTest{
		{name: `good-stores/reg-extension-dir-root`, size: 1, layout: nil},
		{name: `good-stores/unreg-extension-dir-root`, size: 1, layout: testStoreLayout},
		{name: `good-stores/simple-root`, size: 3, layout: testStoreLayout},
		{name: `good-stores/fedora-root.zip`, size: 176, layout: testStoreLayout},
		{name: `bad-stores/E072_root_with_file_not_in_object`, size: 1, layout: testStoreLayout},
		{name: `bad-stores/E073_root_with_empty_dir.zip`, size: 0, layout: testStoreLayout},
	}
	optTable := map[string]*ocflv1.ScanObjectsOpts{
		`default`:       nil,
		`validate`:      {Strict: true},
		`no-validate`:   {Strict: false},
		`fast`:          {Concurrency: 16},
		`slow`:          {Concurrency: 1},
		`fast-validate`: {Strict: true, Concurrency: 16},
		`slow-validate`: {Strict: true, Concurrency: 1},
		// `w-timeout`: {Timeout: time.Microsecond}, // not sure how to test this..
	}
	for mode, opt := range optTable {
		t.Run(mode, func(t *testing.T) {
			for _, sttest := range storeTests {
				t.Run(sttest.name, func(t *testing.T) {
					var fsys fs.FS
					var root string
					var store *ocflv1.Store
					var err error
					var expectErr bool
					if opt != nil && opt.Strict && strings.HasPrefix(sttest.name, "bad-stores") {
						expectErr = true
					}
					if strings.HasSuffix(sttest.name, `.zip`) {
						root = "."
						zreader, err := zip.OpenReader(filepath.Join(storePath, sttest.name))
						if err != nil {
							t.Fatal(err)
						}
						defer zreader.Close()
						fsys = zreader
					} else {
						fsys = os.DirFS(storePath)
						root = sttest.name
					}
					store, err = ocflv1.GetStore(ctx, fsys, root)
					if err != nil {
						t.Fatal(err)
					}
					obj, scanErr := store.ScanObjects(ctx, opt)
					if expectErr {
						if scanErr == nil {
							t.Fatal("expected an error")
						}
						return
					}
					if scanErr != nil {
						t.Fatal(scanErr)
					}
					if l := len(obj); l != sttest.size {
						t.Fatalf("expected %d objects, got %d", sttest.size, l)
					}
					for p := range obj {
						_, err := store.GetPath(ctx, p)
						if err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		})
	}

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
