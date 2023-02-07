package ocflv1_test

import (
	"context"
	"errors"
	"fmt"
	"path"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
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
	ctx := context.Background()
	fsys := ocfl.DirFS(goodObjPath)
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
			t.Run("empty stage", func(t *testing.T) {
				stage := ocfl.NewStage(inv.Alg())
				nextVersionInventoryTest(t, inv, stage, "new version", time.Now(), &ocflv1.User{Name: "Me", Address: "email:me@me.com"})
			})
			t.Run("newfile", func(t *testing.T) {
				stage := ocfl.NewStage(inv.Alg())
				stage.UnsafeAdd("newfile.txt", "content.txt", digest.Set{inv.DigestAlgorithm: "abcd"})
				nextVersionInventoryTest(t, inv, stage, "new version", time.Now(), &ocflv1.User{Name: "Me", Address: "email:me@me.com"})
			})
			t.Run("re-add-digest", func(t *testing.T) {
				// add file with lowercase digest, then remove it, then add it back as uppercase.
				// this is meant to test merging a stage with a source path and a digest that already exists in the inventory.
				// stage 1 -- new file, whacky digest format
				stage1 := ocfl.NewStage(inv.Alg())
				stage1.UnsafeAdd("newfile.txt", "content.txt", digest.Set{inv.DigestAlgorithm: "aBcD"})
				newInv, err := inv.NextVersionInventory(stage1, time.Now(), "add a new file", &ocflv1.User{Name: "Me", Address: "email:me@me.com"})
				if err != nil {
					t.Fatal(err)
				}
				// stage 2 -- remove everything
				stage2 := ocfl.NewStage(inv.Alg())
				newInv, err = newInv.NextVersionInventory(stage2, time.Now(), "remove the file", &ocflv1.User{Name: "Me", Address: "email:me@me.com"})
				if err != nil {
					t.Fatal(err)
				}
				// stage 3 -- readd the file, with uppercase
				stage3 := ocfl.NewStage(inv.Alg())
				stage3.UnsafeAdd("newfile.txt", "content.txt", digest.Set{inv.DigestAlgorithm: "ABCD"})
				nextVersionInventoryTest(t, newInv, stage2, "update again", time.Now(), &ocflv1.User{Name: "Me", Address: "email:me@me.com"})
			})

		})

	}
}

// complete test for NextVersionInventory
func nextVersionInventoryTest(t *testing.T, inv *ocflv1.Inventory, stage *ocfl.Stage, msg string, created time.Time, user *ocflv1.User) *ocflv1.Inventory {
	newInv, err := inv.NextVersionInventory(stage, created, msg, user)
	if err != nil {
		t.Fatal(err)
	}
	isNil(t, newInv.Validate().Err(), "new inventory is invalid")
	isEq(t, newInv.ContentDirectory, inv.ContentDirectory, "new inventory content directory")
	isEq(t, newInv.ID, inv.ID, "new inventory object id")
	isEq(t, newInv.Head.Num(), inv.Head.Num()+1, "new inventory head")
	isEq(t, newInv.Type, inv.Type, "new inventory type field")
	isEq(t, len(newInv.Versions), len(inv.Versions)+1, "number of versions in new inventory")
	// check all manifest entries in old inv are also in new inv
	inv.Manifest.EachPath(func(name string, digest string) error {
		// digests in new inventory are always lowercase
		isEq(t, newInv.Manifest.GetDigest(name), strings.ToLower(digest), "manifest entry for", name)
		return nil
	})
	// check all versions in old inv are in new
	for num, expVer := range inv.Versions {
		t.Run("version-"+num.String(), func(t *testing.T) {
			gotVer := newInv.Versions[num]
			isEq(t, gotVer.Created, expVer.Created, "created field")
			isEq(t, gotVer.Message, expVer.Message, "message field")
			isEq(t, gotVer.User, expVer.User, "user")
			isEq(t, len(gotVer.State.AllPaths()), len(expVer.State.AllPaths()), "state's number of paths")
			expVer.State.EachPath(func(name string, digest string) error {
				isEq(t, gotVer.State.GetDigest(name), strings.ToLower(digest), "state for path", name)
				return nil
			})
		})
	}
	// check the new version
	headVer := newInv.Versions[newInv.Head]
	isEq(t, headVer.Created.Unix(), created.Unix(), "new version timestamp (unix)")
	isEq(t, headVer.Message, msg, "new version message")
	isEq(t, headVer.User, user, "new version user")
	stage.Walk(func(name string, node *ocfl.Index) error {
		if node.IsDir() {
			return nil
		}
		gotdigest := headVer.State.GetDigest(name)
		expDigest := strings.ToLower(node.Val().Digests[inv.DigestAlgorithm])
		isNot(t, gotdigest, "", fmt.Sprintf("new version state is missing '%s'", name))
		isEq(t, gotdigest, expDigest, fmt.Sprintf("new version state is missing '%s'", name))
		// every source path in the stage should be added to the manifest
		// with the version content directory as a prefix
		for _, src := range node.Val().SrcPaths {
			expPath := path.Join(newInv.Head.String(), newInv.ContentDirectory, src)
			isNot(t, newInv.Manifest.GetDigest(expPath), "missing manifest entry for", src)
		}
		return nil
	})
	return newInv
}

func isEq(t *testing.T, got any, exp any, desc ...string) {
	if !reflect.DeepEqual(got, exp) {
		t.Fatalf("%s: got='%v', expect='%v'", fmt.Sprint(desc), got, exp)
	}
}

func isNot(t *testing.T, got any, exp any, desc ...string) {
	if reflect.DeepEqual(got, exp) {
		t.Fatalf("%s: val='%v'", fmt.Sprint(desc), got)
	}
}

func isNil(t *testing.T, v any, desc ...string) {
	if v != nil {
		t.Fatalf("%s: '%v'", fmt.Sprint(desc), v)
	}
}
