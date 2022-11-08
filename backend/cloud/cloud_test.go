package cloud_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend/cloud"
	"github.com/srerickson/ocfl/ocflv1"
	"gocloud.dev/blob"
	"gocloud.dev/blob/fileblob"
	"gocloud.dev/blob/memblob"
)

var (
	testDataPath = filepath.Join(`..`, `..`, `testdata`)
	objPath      = filepath.Join(testDataPath, `object-fixtures`, `1.0`, `good-objects`, `spec-ex-full`)
)

// memBucket
func memBucket(keys map[string][]byte) *blob.Bucket {
	buck := memblob.OpenBucket(nil)
	for k, v := range keys {
		err := buck.WriteAll(context.Background(), k, v, nil)
		if err != nil {
			panic(err)
		}
	}
	return buck
}

func TestOpenFile(t *testing.T) {
	b, err := fileblob.OpenBucket(objPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	buck := cloud.NewFS(b)
	t.Run("invalid path (..)", func(t *testing.T) {
		_, err := buck.OpenFile(context.Background(), `..`)
		if err == nil {
			t.Fatal("expected an error")
		}
		var pErr *fs.PathError
		if !errors.As(err, &pErr) {
			t.Fatal("expected error to be fs.PathError")
		}
	})
	t.Run("invalid path (empty)", func(t *testing.T) {
		_, err := buck.OpenFile(context.Background(), ``)
		if err == nil {
			t.Fatal("expected an error")
		}
		var pErr *fs.PathError
		if !errors.As(err, &pErr) {
			t.Fatal("expected error to be fs.PathError")
		}
	})
	t.Run("existing inventory file", func(t *testing.T) {
		f, err := buck.OpenFile(context.Background(), `inventory.json`)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		_, result := ocflv1.ValidateInventoryReader(context.Background(), f, nil)
		if err := result.Err(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("non-existing file", func(t *testing.T) {
		_, err := buck.OpenFile(context.Background(), `missing.json`)
		if err == nil {
			t.Fatal("expected an error")
		}
		var pErr *fs.PathError
		if !errors.As(err, &pErr) {
			t.Fatal("expected error to be fs.PathError")
		}
	})
	t.Run("directory", func(t *testing.T) {
		_, err := buck.OpenFile(context.Background(), `v1`)
		if err == nil {
			t.Fatal("expected an error")
		}
		var pErr *fs.PathError
		if !errors.As(err, &pErr) {
			t.Fatal("expected error to be fs.PathError")
		}
	})
}

func TestReadDir(t *testing.T) {
	b, err := fileblob.OpenBucket(objPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	buck := cloud.NewFS(b)
	t.Run("invalid path (..)", func(t *testing.T) {
		_, err := buck.OpenFile(context.Background(), `..`)
		if err == nil {
			t.Fatal("expected an error")
		}
		var pErr *fs.PathError
		if !errors.As(err, &pErr) {
			t.Fatal("expected error to be fs.PathError")
		}
	})
	t.Run("invalid path (empty)", func(t *testing.T) {
		_, err := buck.OpenFile(context.Background(), ``)
		if err == nil {
			t.Fatal("expected an error")
		}
		var pErr *fs.PathError
		if !errors.As(err, &pErr) {
			t.Fatal("expected error to be fs.PathError")
		}
	})
	t.Run("top-level directory", func(t *testing.T) {
		entries, err := buck.ReadDir(context.Background(), `.`)
		if err != nil {
			t.Fatal(err)
		}
		info := ocfl.ObjInfoFromFS(entries)
		if info.VersionDirs.Head() != ocfl.V(3) {
			t.Errorf("expected readdir results to include v3, got %v", entries)
		}
		if info.Declaration.Type != "ocfl_object" {
			t.Errorf("expected readdir results to include namasted, got %v", entries)
		}
	})
	t.Run("sub-directory", func(t *testing.T) {
		entries, err := buck.ReadDir(context.Background(), `v1`)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 0 {
			t.Fatal("expected directory entries")
		}
		for _, e := range entries {
			switch e.Name() {
			case "inventory.json":
				if e.IsDir() {
					t.Fatalf("expected %s to be a file", e.Name())
				}
			case "inventory.json.sha512":
				if e.IsDir() {
					t.Fatalf("expected %s to be a file", e.Name())
				}
			case "content":
				if !e.IsDir() {
					t.Fatalf("expected %s to be a directory", e.Name())
				}
			default:
				t.Fatalf("unexpected entry: %s", e.Name())
			}
		}
	})
	t.Run("existing file", func(t *testing.T) {
		_, err := buck.ReadDir(context.Background(), `inventory.json`)
		if err == nil {
			t.Fatal("expected an error")
		}
		var pErr *fs.PathError
		if !errors.As(err, &pErr) {
			t.Fatal("expected error to be fs.PathError")
		}
		if !errors.Is(pErr.Err, fs.ErrNotExist) {
			t.Fatal("expected error to be wrap fs.ErrNotExist")
		}
	})
	t.Run("non-existing (prefix of existing)", func(t *testing.T) {
		_, err := buck.ReadDir(context.Background(), `v`)
		if err == nil {
			t.Fatal("expected an error")
		}
		var pErr *fs.PathError
		if !errors.As(err, &pErr) {
			t.Fatal("expected error to be fs.PathError")
		}
		if !errors.As(err, &pErr) {
			t.Fatal("expected error to be fs.PathError")
		}
		if !errors.Is(pErr.Err, fs.ErrNotExist) {
			t.Fatal("expected error to be wrap fs.ErrNotExist")
		}
	})
	t.Run("very large directory", func(t *testing.T) {
		const num = 10_001
		keys := make(map[string][]byte, num)
		for i := 0; i < num; i++ {
			key := fmt.Sprintf("dir/file-%d.txt", i)
			keys[key] = []byte(key)
		}
		b := memBucket(keys)
		defer b.Close()
		bucket := cloud.NewFS(b)
		entries, err := bucket.ReadDir(context.Background(), "dir")
		if err != nil {
			t.Fatal(err)
		}
		if l := len(entries); l != num {
			t.Fatalf("expected %d entries; got %d", num, l)
		}
	})
}
