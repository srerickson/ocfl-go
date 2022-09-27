package ocflv1_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/internal/testfs"
	"github.com/srerickson/ocfl/ocflv1"
)

func stageFS() ocfl.FS {
	fsys := fstest.MapFS{
		`src1/tmp.txt`:       &fstest.MapFile{Data: []byte(`content1`)},
		`src2/tmp.txt`:       &fstest.MapFile{Data: []byte(`content2`)},
		`src3/a/tmp.txt`:     &fstest.MapFile{Data: []byte(`content2`)},
		`src3/b/another.txt`: &fstest.MapFile{Data: []byte(`another`)},
		`src4/b/another.txt`: &fstest.MapFile{Data: []byte(`another`)},
	}
	return ocfl.NewFS(fsys)
}

func TestStage(t *testing.T) {
	storePath := "test-stage"
	ctx := context.Background()
	fsys := testfs.NewMemFS()
	// initialize store
	if err := ocflv1.InitStore(ctx, fsys, storePath, nil); err != nil {
		t.Fatal(err)
	}
	store, err := ocflv1.GetStore(ctx, fsys, storePath, nil)
	if err != nil {
		t.Fatal(err)
	}

	// v1
	stage, err := store.StageNew(ctx, "object-1", ocflv1.StageFS(stageFS()))
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.AddDir(ctx, `src1`); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(ctx, stage); err != nil {
		t.Fatal(err)
	}
	obj, err := store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := obj.Validate(ctx); err != nil {
		t.Fatal(err)
	}

	// v2
	stage, err = store.StageNext(ctx, obj, ocflv1.StageFS(stageFS()))
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.AddDir(ctx, `src2`); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(ctx, stage); err != nil {
		t.Fatal(err)
	}
	obj, err = store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := obj.Validate(ctx); err != nil {
		t.Fatal(err)
	}

	// v3
	stage, err = store.StageNext(ctx, obj, ocflv1.StageFS(stageFS()))
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.AddDir(ctx, `src3`); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(ctx, stage); err != nil {
		t.Fatal(err)
	}
	obj, err = store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := obj.Validate(ctx); err != nil {
		t.Fatal(err)
	}

	// v4
	stage, err = store.StageNext(ctx, obj, ocflv1.StageFS(stageFS()))
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.AddDir(ctx, `src4`); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(ctx, stage); err != nil {
		t.Fatal(err)
	}
	obj, err = store.GetObject(ctx, "object-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := obj.Validate(ctx); err != nil {
		t.Fatal(err)
	}

	inv, err := obj.Inventory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	len := len(inv.VState(ocfl.VNum{}).State)
	if len != 1 {
		t.Fatalf("expected v4 state to include 1 path not %d", len)
	}
}
