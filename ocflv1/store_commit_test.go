package ocflv1_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend/memfs"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/ocflv1"
)

func TestStoreCommit(t *testing.T) {
	storePath := "test-stage"
	ctx := context.Background()
	storeFS := memfs.New() // store
	stageContent := fstest.MapFS{
		`stage1/tmp.txt`:       &fstest.MapFile{Data: []byte(`content1`)},
		`stage3/a/tmp.txt`:     &fstest.MapFile{Data: []byte(`content2`)},
		`stage3/a/another.txt`: &fstest.MapFile{Data: []byte(`content3`)},
	}
	// stage1 commit is from storeFS, copy files
	for n := range stageContent {
		if !strings.HasPrefix(n, "stage1") {
			continue
		}
		f, err := stageContent.Open(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := storeFS.Write(ctx, n, f); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	// initialize store
	if err := ocflv1.InitStore(ctx, storeFS, storePath, nil); err != nil {
		t.Fatal(err)
	}
	store, err := ocflv1.GetStore(ctx, storeFS, storePath)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("without options", func(t *testing.T) {
		stage := &ocfl.Stage{Alg: digest.SHA256()} // empty stage
		if err = store.Commit(ctx, "object-0", stage); err != nil {
			t.Fatal(err)
		}
		obj, err := store.GetObject(ctx, "object-0")
		if err != nil {
			t.Fatal(err)
		}
		if err := obj.Validate(ctx).Err(); err != nil {
			t.Fatal(err)
		}
	})

	// v1 - add one file "tmp.txt"
	stage1, err := ocfl.NewStage(digest.SHA256(), digest.Map{}, storeFS)
	if err != nil {
		t.Fatal(err)
	}
	if err := stage1.AddRoot(ctx, "stage1"); err != nil {
		t.Fatal(err)
	}
	if stage1.GetStateDigest("tmp.txt") == "" {
		t.Fatal("missing expected digest")
	}
	if err = store.Commit(ctx, "object-1", stage1,
		ocflv1.WithContentDir("foo"),
		ocflv1.WithVersionPadding(2),
		ocflv1.WithUser(ocflv1.User{Name: "Will", Address: "mailto:Will@email.com"}),
		ocflv1.WithMessage("first commit"),
	); err != nil {
		t.Fatal(err)
	}

	// stage 2 - remove "tmp.txt"
	obj, err := store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal()
	}
	state, err := obj.ObjectState(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	stage2, err := ocfl.NewStage(state.Alg, state.Map, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := stage2.RemovePath("tmp.txt"); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(ctx, "object-1", stage2,
		ocflv1.WithUser(ocflv1.User{Name: "Wanda", Address: "mailto:wanda@email.com"}),
		ocflv1.WithMessage("second commit")); err != nil {
		t.Fatal(err)
	}

	// v3 - new files and rename one
	_, err = store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal(err)
	}
	stage3, err := ocfl.NewStage(digest.SHA256(), digest.Map{}, ocfl.NewFS(stageContent))
	if err != nil {
		t.Fatal(err)
	}
	if err = stage3.AddRoot(ctx, "stage3"); err != nil {
		t.Fatal(err)
	}
	// rename one of the staged files
	if err := stage3.RenamePath("a/tmp.txt", "tmp.txt"); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(ctx, "object-1", stage3,
		ocflv1.WithUser(ocflv1.User{Name: "Woody", Address: "mailto:Woody@email.com"}),
		ocflv1.WithMessage("third commit"),
	); err != nil {
		t.Fatal(err)
	}

	// v4 - update one of the files by writing
	obj, err = store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal(err)
	}
	objState, err := obj.ObjectState(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	stage4fsys := memfs.New()
	if _, err := stage4fsys.Write(ctx, "a/another.txt", strings.NewReader("fresh deats")); err != nil {
		t.Fatal(err)
	}
	stage4, err := ocfl.NewStage(objState.Alg, objState.Map, stage4fsys)
	if err != nil {
		t.Fatal(err)
	}
	if err := stage4.AddPath(ctx, "a/another.txt"); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(ctx, "object-1", stage4,
		ocflv1.WithUser(ocflv1.User{Name: "Winnie", Address: "mailto:Winnie@no.com"}),
		ocflv1.WithMessage("last commit"),
	); err != nil {
		t.Fatal(err)
	}

	// validate
	obj, err = store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal(err)
	}
	if result := obj.Validate(ctx); result.Err() != nil {
		t.Fatal("object is invalid", result.Err())
	}
	inv, err := obj.Inventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if inv.ContentDirectory != "foo" {
		t.Fatal("expected foo")
	}
	if inv.DigestAlgorithm != digest.SHA256id {
		t.Fatalf("expected sha256")
	}
	if inv.Head.Padding() != 2 {
		t.Fatalf("expected 2")
	}
	if inv.Head.Num() != 4 {
		t.Fatal("expected 4")
	}
	for num, ver := range inv.Versions {
		if ver.User == nil || ver.User.Name == "" {
			t.Fatal("expected a user for version", num)
		}
		if ver.Message == "" {
			t.Fatal("expected a message for version", num)
		}
	}
	finalState, err := obj.ObjectState(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if finalState.GetDigest("tmp.txt") == "" {
		t.Fatal("missing expected file")
	}
	if finalState.GetDigest("a/another.txt") == "" {
		t.Fatal("missing expected file")
	}
	if len(finalState.AllPaths()) != 2 {
		t.Fatal("expected only two items")
	}
}
