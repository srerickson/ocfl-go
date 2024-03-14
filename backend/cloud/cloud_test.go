package cloud_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/cloud"
	"github.com/srerickson/ocfl-go/ocflv1"
	"gocloud.dev/blob"
	"gocloud.dev/blob/fileblob"
	"gocloud.dev/blob/memblob"
)

var _ ocfl.ObjectRootIterator = (*cloud.FS)(nil)

var (
	testDataPath = filepath.Join(`..`, `..`, `testdata`)
	goodObjects  = filepath.Join(testDataPath, `object-fixtures`, `1.0`, `good-objects`)
	warnObjects  = filepath.Join(testDataPath, `object-fixtures`, `1.0`, `warn-objects`)
	objPath      = filepath.Join(goodObjects, `spec-ex-full`)
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
		_, result := ocflv1.ValidateInventoryReader(context.Background(), f)
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
	ctx := context.Background()
	b, err := fileblob.OpenBucket(objPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	buck := cloud.NewFS(b)
	t.Run("invalid path (..)", func(t *testing.T) {
		_, err := buck.OpenFile(ctx, `..`)
		if err == nil {
			t.Fatal("expected an error")
		}
		var pErr *fs.PathError
		if !errors.As(err, &pErr) {
			t.Fatal("expected error to be fs.PathError")
		}
	})
	t.Run("invalid path (empty)", func(t *testing.T) {
		_, err := buck.OpenFile(ctx, ``)
		if err == nil {
			t.Fatal("expected an error")
		}
		var pErr *fs.PathError
		if !errors.As(err, &pErr) {
			t.Fatal("expected error to be fs.PathError")
		}
	})
	t.Run("top-level directory", func(t *testing.T) {
		obj, err := ocfl.GetObjectRoot(ctx, buck, ".")
		if err != nil {
			t.Fatal(err)
		}
		if v := obj.VersionDirs.Head(); v != ocfl.V(3) {
			t.Errorf("expected readdir results to include v3, got %v", v)
		}
	})
	t.Run("sub-directory", func(t *testing.T) {
		entries, err := buck.ReadDir(ctx, `v1`)
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
		_, err := buck.ReadDir(ctx, `inventory.json`)
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
		_, err := buck.ReadDir(ctx, `v`)
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
		entries, err := bucket.ReadDir(ctx, "dir")
		if err != nil {
			t.Fatal(err)
		}
		if l := len(entries); l != num {
			t.Fatalf("expected %d entries; got %d", num, l)
		}
	})
}

func TestWrite(t *testing.T) {
	ctx := context.Background()
	buck := memBucket(nil)
	fsys := cloud.NewFS(buck)
	type writeTest struct {
		name      string
		cont      string
		expectErr bool
	}
	testTable := map[string]writeTest{
		"single file":        {name: "test.txt", cont: "test data", expectErr: false},
		"single file in dir": {name: "a/b/c/test.txt", cont: "test data", expectErr: false},
		"invalid path ..":    {name: "../test.txt", cont: "test data", expectErr: true},
		"invalid path /":     {name: "/test.txt", cont: "test data", expectErr: true},
		"invalid path ./":    {name: "./test.txt", cont: "test data", expectErr: true},
	}
	for testName, test := range testTable {
		t.Run(testName, func(t *testing.T) {
			size, err := fsys.Write(ctx, test.name, strings.NewReader(test.cont))
			if test.expectErr {
				if err == nil {
					t.Fatal("expected an error, but got none")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if int(size) != len(test.cont) {
				t.Fatalf("expected write to return %d, got %d", len(test.cont), size)
			}
			f, err := fsys.OpenFile(ctx, test.name)
			if err != nil {
				t.Fatal("opening file", err)
			}
			cont, err := io.ReadAll(f)
			if err != nil {
				t.Fatal("reading file", err)
			}
			defer f.Close()
			if string(cont) != test.cont {
				t.Fatalf("'%s' != '%s'", string(cont), test.cont)
			}
		})
	}
}

func TestRemove(t *testing.T) {
	ctx := context.Background()
	type removeTest struct {
		name      string
		expectErr bool
	}
	testTable := map[string]removeTest{
		"single file":     {name: "a/b/c.txt", expectErr: false},
		"not exist":       {name: "a/b/c2.txt", expectErr: true},
		"not file":        {name: "a/b", expectErr: true},
		"invalid path .":  {name: ".", expectErr: true},
		"invalid path ..": {name: "a/../a/b/c.txt", expectErr: true},
		"invalid path /":  {name: "a/a/b/c.txt", expectErr: true},
	}
	for testName, test := range testTable {
		t.Run(testName, func(t *testing.T) {
			buck := memBucket(map[string][]byte{
				"a/b/c.txt": []byte("sample data"),
				"a/b.txt":   []byte("more sample data"),
			})
			fsys := cloud.NewFS(buck)
			err := fsys.Remove(ctx, test.name)
			if test.expectErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRemoveAll(t *testing.T) {
	ctx := context.Background()
	type removeTest struct {
		name      string
		expectErr bool
	}
	testTable := map[string]removeTest{
		"single dir":      {name: "a/b", expectErr: false},
		"not exist":       {name: "a2", expectErr: false},
		"invalid path .":  {name: ".", expectErr: true},
		"invalid path ..": {name: "a/../a/b/c.txt", expectErr: true},
		"invalid path /":  {name: "/a/b/c.txt", expectErr: true},
	}
	for testName, test := range testTable {
		t.Run(testName, func(t *testing.T) {
			buck := memBucket(map[string][]byte{
				"a/b/c.txt": []byte("sample data"),
				"a/b.txt":   []byte("more sample data"),
			})
			fsys := cloud.NewFS(buck)
			err := fsys.RemoveAll(ctx, test.name)
			if test.expectErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCopy(t *testing.T) {
	ctx := context.Background()
	testContent := map[string][]byte{
		"a/b/c.txt": []byte("sample data"),
		"a/b.txt":   []byte("more sample data"),
	}
	type copyTest struct {
		src       string
		dst       string
		expectErr bool
	}
	testTable := map[string]copyTest{
		"basic":             {src: "a/b.txt", dst: "a/b2.txt", expectErr: false},
		"overwrite dst":     {src: "a/b.txt", dst: "a/b/c.txt", expectErr: false},
		"src doesn't exist": {src: "none", dst: "c.txt", expectErr: true},
		"src is empty":      {src: "", dst: "c.txt", expectErr: true},
		"src is invalid":    {src: ".", dst: "c.txt", expectErr: true},
		"dst is empty":      {src: "a/b.txt", dst: "", expectErr: true},
		"dst is invalid":    {src: "a/b.txt", dst: ".", expectErr: true},
	}
	for testName, test := range testTable {
		t.Run(testName, func(t *testing.T) {
			buck := memBucket(testContent)
			fsys := cloud.NewFS(buck)
			err := fsys.Copy(ctx, test.dst, test.src)
			if test.expectErr && err == nil {
				t.Fatal("expected an error")
			}
			if !test.expectErr {
				if err != nil {
					t.Fatal(err)
				}
				dstF, err := fsys.OpenFile(ctx, test.dst)
				if err != nil {
					t.Fatal(err)
				}
				dstByts, err := io.ReadAll(dstF)
				if err != nil {
					t.Fatal(err)
				}
				srcF, err := fsys.OpenFile(ctx, test.src)
				if err != nil {
					t.Fatal(err)
				}
				srcByts, err := io.ReadAll(srcF)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(srcByts, dstByts) {
					t.Fatal("destintation doesn't have samve content as source")
				}
			}
		})
	}
}

func TestObjectRoots(t *testing.T) {
	b, err := fileblob.OpenBucket(warnObjects, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	fsys := cloud.NewFS(b)
	numobjs := 0
	fn := func(obj *ocfl.ObjectRoot) error {
		numobjs++
		if obj.SidecarAlg == "" {
			t.Error("algorithm not set for", obj.Path)
		}
		if !obj.HasInventory() {
			t.Error("HasInventory false for", obj.Path)
		}
		if !obj.HasSidecar() {
			t.Error("HasSidecar false for", obj.Path)
		}
		if err := obj.VersionDirs.Valid(); err != nil {
			t.Error("version dirs not valid for", obj.Path)
		}
		v3Fixture := "w001_zero_padded_versions"
		if strings.HasSuffix(obj.Path, v3Fixture) {
			if len(obj.VersionDirs) != 3 {
				t.Error(obj.Path, "should have 3 versions")
			}
		}
		extFixture := "W013_unregistered_extension"
		if strings.HasSuffix(obj.Path, extFixture) {
			if obj.Flags&ocfl.FoundExtensions == 0 {
				t.Errorf(obj.Path, "should have extensions flag")
			}
		}
		return nil
	}
	err = fsys.ObjectRoots(context.Background(), ocfl.PathSelector{}, fn)
	if err != nil {
		t.Fatal(err)
	}
	if numobjs != 12 {
		t.Fatalf("expected 12 objects to be called, got %d", numobjs)
	}
}

func TestLogging(t *testing.T) {
	ctx := context.Background()
	buff := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(buff, &slog.HandlerOptions{Level: slog.LevelDebug}))
	membuck := memBucket(map[string][]byte{"a/file.txt": []byte("content")})
	fsys := cloud.NewFS(membuck, cloud.WithLogger(logger))
	loggingFuncs := map[string]func(){
		"openfile": func() {
			f, _ := fsys.OpenFile(ctx, `a/file.txt`)
			if f != nil {
				defer f.Close()
			}
		},
		"readdir":     func() { fsys.ReadDir(ctx, `a`) },
		"write":       func() { fsys.Write(ctx, "tmp", strings.NewReader("cont")) },
		"remove":      func() { fsys.Remove(ctx, "a/file.txt") },
		"removeall":   func() { fsys.RemoveAll(ctx, "a") },
		"copy":        func() { fsys.Copy(ctx, "a", "b") },
		"objectroots": func() { fsys.ObjectRoots(ctx, ocfl.Dir("."), func(_ *ocfl.ObjectRoot) error { return nil }) },
	}
	for fnName, fn := range loggingFuncs {
		fn()
		if buff.Len() == 0 {
			t.Error(fnName, "didn't write to logger as expected")
		}
		buff.WriteTo(io.Discard)
	}
}
