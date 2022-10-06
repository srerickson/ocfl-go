package ocflv1_test

import (
	"context"
	"errors"
	"fmt"
	"path"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/ocflv1"
)

func TestInventoryIndex(t *testing.T) {
	fsys := ocfl.DirFS(goodObjPath)
	ctx := context.Background()
	goodObjects, err := fsys.ReadDir(ctx, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range goodObjects {
		t.Run(dir.Name(), func(t *testing.T) {
			conf := ocflv1.ValidateInventoryConf{
				FS:   fsys,
				Name: path.Join(dir.Name(), "inventory.json"),
			}
			inv, err := ocflv1.ValidateInventory(ctx, &conf)
			if err != nil {
				t.Fatal(err)
			}
			tree, err := inv.IndexFull(ocfl.Head, true, true)
			if err != nil {
				t.Fatal(err)
			}
			err = tree.Walk(func(name string, isdir bool, inf *ocfl.IndexItem) error {
				if isdir {
					return nil
				}
				if _, exists := inf.Digests[inv.DigestAlgorithm]; !exists {
					return errors.New("missing inventory's digest alg")
				}
				src, err := inv.ContentPath(ocfl.Head, name)
				if err != nil {
					return err
				}
				if !inf.HasSrc(src) {
					return fmt.Errorf("%v does not include %s", inf.SrcPaths, src)
				}
				if _, exists := inf.Digests[inv.DigestAlgorithm]; !exists {
					return errors.New("missing inventory's digest alg")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})

	}

}
