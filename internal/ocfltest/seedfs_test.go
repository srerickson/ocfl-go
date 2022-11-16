package ocfltest_test

import (
	"bytes"
	"io"
	"io/fs"
	"math/rand"
	"testing"
	"testing/fstest"

	"github.com/srerickson/ocfl/internal/ocfltest"
)

func TestSeedFS(t *testing.T) {
	fsys := ocfltest.SeedFS{
		"b/another.txt":   &ocfltest.SeedFile{Size: 811, Seed: 0},
		"a/file.txt":      &ocfltest.SeedFile{Size: 1024, Seed: 687},
		"b/duplicate.txt": &ocfltest.SeedFile{Size: 1024, Seed: 687},
		"a/b/c/d.txt":     &ocfltest.SeedFile{Size: 1024 * 1024 * 8, Seed: 5},
	}
	// valid implementation of fs.FS
	keys := make([]string, len(fsys))
	i := 0
	for k := range fsys {
		keys[i] = k
		i++
	}
	if err := fstest.TestFS(fsys, keys...); err != nil {
		t.Fatal(err)
	}
	// expected content in files
	for name, f := range fsys {
		buff := &bytes.Buffer{}
		reader := rand.New(rand.NewSource(f.Seed))
		n, err := io.CopyN(buff, reader, f.Size)
		if err != nil {
			t.Fatal(err)
		}
		expBytes := buff.Bytes()[:n]
		fileByts, err := fs.ReadFile(fsys, name)
		if err != nil {
			t.Fatal(err)
		}
		if len(fileByts) != int(f.Size) {
			t.Fatal("file content is not expected size for", name)
		}
		if !bytes.Equal(expBytes, fileByts) {
			t.Fatal("file content is not expected for", name)
		}
	}
}
