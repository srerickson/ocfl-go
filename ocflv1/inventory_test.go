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
			name := path.Join(dir.Name(), "inventory.json")
			inv, result := ocflv1.ValidateInventory(ctx, fsys, name, nil)
			if err := result.Err(); err != nil {
				t.Fatal(err)
			}
			tree, err := inv.Index(ocfl.Head)
			if err != nil {
				t.Fatal(err)
			}
			err = tree.Walk(func(name string, node *ocfl.Index) error {
				if node.IsDir() {
					return nil
				}
				if _, exists := node.Val().Digests[inv.DigestAlgorithm]; !exists {
					return errors.New("missing inventory's digest alg")
				}
				src, err := inv.ContentPath(ocfl.Head, name)
				if err != nil {
					return err
				}
				var exists bool
				for _, s := range node.Val().SrcPaths {
					if s == src {
						exists = true
					}
				}
				if !exists {
					return fmt.Errorf("%v does not include %s", node.Val().SrcPaths, src)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
