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
			t.Run("stage newfile", func(t *testing.T) {
				stage := ocfl.NewStage(inv.Alg())
				stage.UnsafeAdd("newfile.txt", "content.txt", digest.Set{inv.DigestAlgorithm: "abcd"})
				nextVersionInventoryTest(t, inv, stage, "new version", time.Now(), &ocflv1.User{Name: "Me", Address: "email:me@me.com"})
			})
			t.Run("stage newfile, fixity", func(t *testing.T) {
				stage := ocfl.NewStage(inv.Alg())
				stage.UnsafeAdd("newfile.txt", "content.txt", digest.Set{inv.DigestAlgorithm: "abcd", digest.MD5().ID(): "1234"})
				nextVersionInventoryTest(t, inv, stage, "new version", time.Now(), &ocflv1.User{Name: "Me", Address: "email:me@me.com"})
			})
			t.Run("stage re-add-digest", func(t *testing.T) {
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
	// fixity values in old inventory should be in new inventory. A section for
	// the inventory's digest algorithm will never be included in the fixity
	// section of the new inventory even if it exists in the previous inventory.
	for fixalg, fixmap := range inv.Fixity {
		if fixalg == newInv.DigestAlgorithm {
			continue
		}
		t.Run("fixity-"+fixalg, func(t *testing.T) {
			gotfix := newInv.Fixity[fixalg]
			if gotfix == nil {
				t.Fatal("missing in new inventory")
			}
			fixmap.EachPath(func(name string, digest string) error {
				isEq(t, gotfix.GetDigest(name), strings.ToLower(digest), "fixity entry for", name)
				return nil
			})
		})
	}
	// check new version: values from NextVersionInventory found in stage
	headVer := newInv.Versions[newInv.Head]
	isEq(t, headVer.Created.Unix(), created.Unix(), "new version timestamp (unix)")
	isEq(t, headVer.Message, msg, "new version message")
	isEq(t, headVer.User, user, "new version user")
	stage.Walk(func(lgcPath string, node *ocfl.Index) error {
		if node.IsDir() {
			return nil
		}
		stageDigests := node.Val().Digests
		stageDigest := stageDigests[newInv.DigestAlgorithm]
		stateDigest := headVer.State.GetDigest(lgcPath)
		isNot(t, stateDigest, "", fmt.Sprintf("new version state is missing '%s'", lgcPath))
		isEq(t, stateDigest, stageDigest, fmt.Sprintf("wrong digest in version state for '%s'", lgcPath))
		// the stage digest should be associacate with all source paths in the
		// manifest. Every source path in the stage should be added to the
		// manifest with the version content directory as a prefix
		for _, src := range node.Val().SrcPaths {
			expPath := path.Join(newInv.Head.String(), newInv.ContentDirectory, src)
			manifestDigest := newInv.Manifest.GetDigest(expPath)
			isEq(t, manifestDigest, stageDigest, "manifest digest for", expPath)
		}
		// additional digests in the stage should be present in the
		// appropriate fixity block for the new inventory.
		for alg, stageDigest := range node.Val().Digests {
			if alg == newInv.DigestAlgorithm {
				continue
			}
			fixity := newInv.Fixity[alg]
			if fixity == nil {
				t.Fatal("missing fixity for ", alg)
			}
			for _, src := range node.Val().SrcPaths {
				expPath := path.Join(newInv.Head.String(), newInv.ContentDirectory, src)
				fixityDigest := fixity.GetDigest(expPath)
				isEq(t, fixityDigest, strings.ToLower(stageDigest), "fixity digest for", expPath)
			}
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

func isNot(t *testing.T, got any, not any, desc ...string) {
	if reflect.DeepEqual(got, not) {
		t.Fatalf("%s: val='%v'", fmt.Sprint(desc), not)
	}
}

func isNil(t *testing.T, v any, desc ...string) {
	if v != nil {
		t.Fatalf("%s: '%v'", fmt.Sprint(desc), v)
	}
}
