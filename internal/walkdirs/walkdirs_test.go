package walkdirs_test

import (
	"bytes"
	"context"
	"io/fs"
	"reflect"
	"sort"
	"testing"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/memfs"
	"github.com/srerickson/ocfl-go/internal/walkdirs"
)

func newTestFS(data map[string][]byte) ocfl.FS {
	ctx := context.Background()
	fsys := memfs.New()
	for name, file := range data {
		_, err := fsys.Write(ctx, name, bytes.NewBuffer(file))
		if err != nil {
			panic(err)
		}
	}
	return fsys
}

func TestWalkDirs(t *testing.T) {
	ctx := context.Background()
	fsys := newTestFS(map[string][]byte{
		"ignore/file.text":  []byte("content"),
		"a/b/c/d/file.text": []byte("content"),
	})
	t.Run("ignored paths", func(t *testing.T) {
		var walked []string
		walkFn := func(name string, entries []fs.DirEntry, err error) error {
			walked = append(walked, name)
			if name == "a/b/c" {
				return walkdirs.ErrSkipDirs
			}
			return nil
		}
		ignore := func(name string) bool { return name == "ignore" }
		walkdirs.WalkDirs(ctx, fsys, ".", ignore, walkFn, 0)
		sort.Strings(walked)
		expect := []string{".", "a", "a/b", "a/b/c"}
		if !reflect.DeepEqual(walked, expect) {
			t.Fatalf("expected %v, got %v", expect, walked)
		}
	})
	t.Run("lexical order with concurrency=1", func(t *testing.T) {
		lastName := "."
		walkFn := func(name string, entries []fs.DirEntry, err error) error {
			if lastName != "." && lastName > name {
				t.Fatalf("out of order walk: %s before %s", lastName, name)
			}
			lastName = name
			return nil
		}
		walkdirs.WalkDirs(ctx, fsys, ".", nil, walkFn, 1)
	})

}
