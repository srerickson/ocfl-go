package ocflv1_test

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/ocflv1"
)

func TestScanObjects(t *testing.T) {
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
					var fsys ocfl.FS
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
					scanFn := func(obj *ocflv1.Object) error {
						if _, err := obj.Inventory(ctx); err != nil {
							t.Fatal(err)
						}
						numObjs++
						return nil
					}
					scanErr := store.ScanObjects(ctx, scanFn, opt)
					if expectErr {
						if scanErr == nil {
							t.Fatal("expected an error")
						}
						return
					}
					if scanErr != nil {
						t.Fatal(scanErr)
					}
					if numObjs != sttest.size {
						t.Fatalf("expected %d objects, got %d", sttest.size, numObjs)
					}
				})
			}
		})
	}
}
