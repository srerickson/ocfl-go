package ocflv1_test

import (
	"archive/zip"
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srerickson/ocfl/namaste"
	store "github.com/srerickson/ocfl/ocflv1"
)

func TestScanObjects(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(`testdata`, `store-fixtures`, `1.0`)
	// map to store to expected # of objects
	testStore := map[string]int{
		`good-stores/reg-extension-dir-root`:           1,
		`good-stores/unreg-extension-dir-root`:         1,
		`good-stores/simple-root`:                      3,
		`good-stores/fedora-root.zip`:                  176,
		`bad-stores/E072_root_with_file_not_in_object`: 1,
		`bad-stores/E073_root_with_empty_dir.zip`:      0,
	}
	modes := map[string]*store.ScanObjectsOpts{
		`default`:       nil,
		`validate`:      {Strict: true},
		`no-validate`:   {Strict: false},
		`fast`:          {Concurrency: 16},
		`slow`:          {Concurrency: 1},
		`fast-validate`: {Strict: true, Concurrency: 16},
		`slow-validate`: {Strict: true, Concurrency: 1},
		// `w-timeout`: {Timeout: time.Microsecond}, // not sure how to test this..
	}
	for mode, opt := range modes {
		t.Run(mode, func(t *testing.T) {
			for name, num := range testStore {
				t.Run(name, func(t *testing.T) {
					var fsys fs.FS
					var root string
					var expectErr bool
					if opt != nil && opt.Strict && strings.HasPrefix(name, "bad-stores") {
						expectErr = true
					}
					if strings.HasSuffix(name, `.zip`) {
						root = "."
						zreader, err := zip.OpenReader(filepath.Join(storePath, name))
						if err != nil {
							t.Fatal(err)
						}
						defer zreader.Close()
						fsys = zreader
					} else {
						fsys = os.DirFS(storePath)
						root = name
					}
					obj, scanErr := store.ScanObjects(ctx, fsys, root, opt)
					if expectErr {
						if scanErr == nil {
							t.Fatal("expected an error")
						}
						return
					}
					if scanErr != nil {
						t.Fatal(scanErr)
					}
					if l := len(obj); l != num {
						t.Fatalf("expected %d objects, got %d", num, l)
					}
					for p, v := range obj {
						decl := namaste.Declaration{
							Type:    namaste.ObjectType,
							Version: v,
						}.Name()
						f, err := fsys.Open(path.Join(root, p, decl))
						if err != nil {
							t.Fatal(err)
						}
						defer f.Close()
						if _, err = f.Stat(); err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		})
	}

}
