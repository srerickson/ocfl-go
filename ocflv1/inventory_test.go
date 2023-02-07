package ocflv1_test

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/internal/testfs"
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

func TestInventorNextVersionInventory(t *testing.T) {
	fsys := ocfl.DirFS(goodObjPath)
	ctx := context.Background()
	goodObjects, err := fsys.ReadDir(ctx, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range goodObjects {
		t.Run(dir.Name(), func(t *testing.T) {
			ctx := context.Background()
			name := path.Join(dir.Name(), "inventory.json")
			inv, result := ocflv1.ValidateInventory(ctx, fsys, name, nil)
			if err := result.Err(); err != nil {
				t.Fatal(err)
			}
			// build a new inventory from modified stage
			stagefs := testfs.NewMemFS()
			idx, _ := inv.Index(ocfl.VNum{})
			stage := ocfl.NewStage(inv.Alg(), ocfl.StageIndex(idx), ocfl.StageRoot(stagefs, "."))
			if _, err := stage.WriteFile(ctx, "newfile.txt", strings.NewReader("new file content")); err != nil {
				t.Fatal(err)
			}
			created := time.Now()
			msg := "new version"
			user := &ocflv1.User{Name: "Me", Address: "email:me@me.com"}
			newInv, err := inv.NextVersionInventory(stage, created, msg, user)
			if err != nil {
				t.Fatal(err)
			}
			// new inventory is valid
			if err := newInv.Validate().Err(); err != nil {
				t.Fatal("new inventory is not valid", err)
			}
			// new inventory has one more version than previous
			if len(newInv.Versions) != len(inv.Versions)+1 {
				t.Fatal("new inventory doesn't have additional version")
			}
			ver := newInv.Versions[newInv.Head]
			if ver.Created.Unix() != created.Unix() {
				t.Fatal("new version doesn't have right created timestamp")
			}
			if ver.Message != msg {
				t.Fatal("new version doesn't have right message")
			}
			if ver.User != user {
				t.Fatal("new version doesn't have right user")
			}
		})
	}
}
