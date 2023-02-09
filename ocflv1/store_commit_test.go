package ocflv1_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/testfs"
	"github.com/srerickson/ocfl/ocflv1"
)

func TestStoreCommit(t *testing.T) {
	storePath := "test-stage"
	ctx := context.Background()
	storeFS := testfs.NewMemFS() // store
	stgFS := ocfl.NewFS(fstest.MapFS{
		`stage1/tmp.txt`:       &fstest.MapFile{Data: []byte(`content1`)},
		`stage3/a/tmp.txt`:     &fstest.MapFile{Data: []byte(`content2`)},
		`stage3/a/another.txt`: &fstest.MapFile{Data: []byte(`content3`)},
	})

	// initialize store
	if err := ocflv1.InitStore(ctx, storeFS, storePath, nil); err != nil {
		t.Fatal(err)
	}
	store, err := ocflv1.GetStore(ctx, storeFS, storePath)
	if err != nil {
		t.Fatal(err)
	}

	// v1 - add one file "tmp.txt"
	stage1 := ocfl.NewStage(digest.SHA256(), ocfl.StageRoot(stgFS, `stage1`))
	if err := stage1.AddAllFromRoot(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := stage1.GetVal("tmp.txt"); err != nil {
		t.Fatal(err)
	}
	if err = store.Commit(ctx, "object-1", stage1,
		ocflv1.WithContentDir("foo"),
		ocflv1.WithVersionPadding(2),
		ocflv1.WithUser("Will", "mailto:Will@email.com"),
		ocflv1.WithMessage("first commit"),
	); err != nil {
		t.Fatal(err)
	}

	// stage 2 - remove "tmp.txt"
	obj, err := store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal()
	}
	stage2, err := obj.NewStage(ctx, ocfl.Head)
	if err != nil {
		t.Fatal()
	}
	if err := stage2.Remove("tmp.txt"); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(ctx, "object-1", stage2,
		ocflv1.WithUser("Wanda", "mailto:wanda@email.com"),
		ocflv1.WithMessage("second commit")); err != nil {
		t.Fatal(err)
	}

	// v3 - new files and rename one
	obj, err = store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal(err)
	}
	stage3, err := obj.NewStage(ctx, ocfl.Head, ocfl.StageRoot(stgFS, `stage3`))
	if err != nil {
		t.Fatal(err)
	}
	if err := stage3.AddAllFromRoot(ctx); err != nil {
		t.Fatal(err)
	}
	// rename one of the staged files
	if err := stage3.Rename("a/tmp.txt", "tmp.txt"); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(ctx, "object-1", stage3,
		ocflv1.WithUser("Woody", "mailto:Woody@email.com"),
		ocflv1.WithMessage("third commit"),
	); err != nil {
		t.Fatal(err)
	}

	// v4 - update one of the files by writing
	obj, err = store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal(err)
	}
	stage4, err := obj.NewStage(ctx, ocfl.Head, ocfl.StageRoot(testfs.NewMemFS(), `.`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stage4.WriteFile(ctx, "a/another.txt", strings.NewReader("fresh deats")); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(ctx, "object-1", stage4,
		ocflv1.WithUser("Winnie", "mailto:Winnie@no.com"),
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
	idx, err := inv.Index(ocfl.Head.Num())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := idx.GetVal("tmp.txt"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := idx.GetVal("a/another.txt"); err != nil {
		t.Fatal(err)
	}
	if idx.Len() != 2 {
		t.Fatal("expected only two items")
	}
}
