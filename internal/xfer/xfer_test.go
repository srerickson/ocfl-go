package xfer_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/testfs"
	"github.com/srerickson/ocfl/internal/xfer"
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
	dst := testfs.NewMemFS()
	for f, c := range files {
		_, err := dst.Write(ctx, f, strings.NewReader(c))
		if err != nil {
			return nil, err
		}
	}
	return dst, nil
}

func TestXfer(t *testing.T) {
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
	algs := []digest.Alg{digest.MD5(), digest.SHA256()}
	result, err := xfer.DigestXfer(ctx, src, dst, files, xfer.WithAlgs(algs...), xfer.WithGoNums(2))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(result)
}
