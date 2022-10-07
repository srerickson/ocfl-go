package ocflv1_test

import (
	"archive/zip"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/digest/checksum"
	"github.com/srerickson/ocfl/extensions"
	"github.com/srerickson/ocfl/internal/testfs"
	"github.com/srerickson/ocfl/ocflv1"
)

var storePath = filepath.Join(`..`, `testdata`, `store-fixtures`, `1.0`)

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
			scan, err := store.ScanObjects(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			for p := range scan {
				obj, err := store.GetObjectPath(ctx, p)
				if err != nil {
					t.Fatal(err)
				}
				inv, err := obj.Inventory(ctx)
				if err != nil {
					t.Fatal(err)
				}
				// test layout works
				_, err = store.GetObject(ctx, inv.ID)
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
						_, err := store.GetObjectPath(ctx, p)
						if err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		})
	}
}

func TestStoreUpdateObject(t *testing.T) {
	storePath := "test-stage"
	ctx := context.Background()
	storeFS := testfs.NewMemFS() // store
	stgFS := stageFS()
	// initialize store
	if err := ocflv1.InitStore(ctx, storeFS, storePath, nil); err != nil {
		t.Fatal(err)
	}
	store, err := ocflv1.GetStore(ctx, storeFS, storePath)
	if err != nil {
		t.Fatal(err)
	}

	// v1
	stage, err := ocfl.IndexDir(ctx, stgFS, `src1`, checksum.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := stage.Get("tmp.txt"); err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(ctx, "object-1", stage,
		ocflv1.WithAlg(digest.SHA256),
		ocflv1.WithContentDir("foo"),
		ocflv1.WithVersionPadding(2),
		ocflv1.WithUser("Bill", "mailto:me@no.com"),
		ocflv1.WithMessage("first commit"),
	); err != nil {
		t.Fatal(err)
	}
	obj, err := store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := obj.Validate(ctx); err != nil {
		t.Fatal("object is invalid", err)
	}
	inv, err := obj.Inventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if inv.ContentDirectory != "foo" {
		t.Fatal("expected foo")
	}
	if inv.DigestAlgorithm != digest.SHA256 {
		t.Fatalf("expected sha256")
	}
	if inv.Head.Padding() != 2 {
		t.Fatalf("expected 2")
	}
	if u := inv.Versions[inv.Head].User; u == nil || u.Name != "Bill" {
		t.Fatal("expected Bill")
	}
	if u := inv.Versions[inv.Head].User; u == nil || u.Address != "mailto:me@no.com" {
		t.Fatal("expected Bill")
	}
	if inv.Versions[inv.Head].Message != "first commit" {
		t.Fatal("expected 'first commit'")
	}
	stage2, err := inv.IndexFull(ocfl.Head, true, false)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := stage.Diff(stage2, digest.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.Equal() {
		t.Fatal("expected head state to be the same as the stage state")
	}

	//v2 - empty
	if _, err := stage2.Remove("tmp.txt", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(ctx, "object-1", stage2); err != nil {
		t.Fatal(err)
	}
	obj, err = store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := obj.Validate(ctx); err != nil {
		t.Fatal(err)
	}
	inv, err = obj.Inventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	head, err := inv.Index(ocfl.Head)
	if err != nil {
		t.Fatal(err)
	}
	if head.Len() != 0 {
		t.Fatal("expected head state to be empty")
	}

	// v3
	stage3, err := ocfl.IndexDir(ctx, stgFS, "src2", checksum.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(ctx, "object-1", stage3); err != nil {
		t.Fatal(err)
	}
	obj, err = store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := obj.Validate(ctx); err != nil {
		t.Fatal(err)
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

func stageFS() ocfl.FS {
	fsys := fstest.MapFS{
		`src1/tmp.txt`:       &fstest.MapFile{Data: []byte(`content1`)},
		`src2/a/tmp.txt`:     &fstest.MapFile{Data: []byte(`content2`)},
		`src3/a/tmp.txt`:     &fstest.MapFile{Data: []byte(`content3`)},
		`src3/b/another.txt`: &fstest.MapFile{Data: []byte(`another`)},
		`src4/b/another.txt`: &fstest.MapFile{Data: []byte(`another`)},
	}
	return ocfl.NewFS(fsys)
}
