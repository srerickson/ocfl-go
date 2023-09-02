package xfer_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/memfs"
	"github.com/srerickson/ocfl-go/internal/xfer"
)

func srcFS(files map[string]string) ocfl.FS {
	src := fstest.MapFS{}
	for f, c := range files {
		src[f] = &fstest.MapFile{Data: []byte(c)}
	}
	return ocfl.NewFS(src)
}

func dstFS(files map[string]string) (ocfl.WriteFS, error) {
	ctx := context.Background()
	dst := memfs.New()
	for f, c := range files {
		_, err := dst.Write(ctx, f, strings.NewReader(c))
		if err != nil {
			return nil, err
		}
	}
	return dst, nil
}

func TestCopy(t *testing.T) {
	ctx := context.Background()
	src := srcFS(map[string]string{
		"file.txt":  "content",
		"file2.txt": "content",
		"file3.txt": "content",
		"file4.txt": "content",
	})
	dst, err := dstFS(nil)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"file.txt":  "file.txt",
		"file2.txt": "file2.txt",
		"file3.txt": "file3.txt",
		"file4.txt": "file4.txt",
	}
	if err := xfer.Copy(ctx, src, dst, files, 2); err != nil {
		t.Fatal(err)
	}
}
