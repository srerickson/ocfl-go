package local_test

import (
	"os"
	"testing"

	"github.com/matryer/is"
	"github.com/srerickson/ocfl/backend/local"
	"github.com/srerickson/ocfl/backend/test"
)

func TestLocalBackend(t *testing.T) {
	is := is.New(t)
	tmpDir, err := os.MkdirTemp("", "testwrite-*")
	is.NoErr(err)
	defer os.RemoveAll(tmpDir)
	bak, err := local.NewBackend(tmpDir)
	is.NoErr(err)
	test.TestBackend(t, bak)
}

// func TestStoreScan(t *testing.T) {
// 	is := is.New(t)
// 	tmp, err := os.MkdirTemp("", "empty-dirst-*")
// 	is.NoErr(err)
// 	defer os.RemoveAll(tmp)

// 	// setup test content
// 	err = os.MkdirAll(filepath.Join(tmp, "bad-1", "b", "c", "d", "e"), 0755)
// 	is.NoErr(err)
// 	err = os.MkdirAll(filepath.Join(tmp, "bad-2", "b"), 0755)
// 	is.NoErr(err)
// 	err = ioutil.WriteFile(filepath.Join(tmp, "bad-2", "b", "c.txt"), []byte("content"), 0644)
// 	is.NoErr(err)
// 	err = os.MkdirAll(filepath.Join(tmp, "good-1", "b", "c"), 0755)
// 	is.NoErr(err)

// 	bak, err := local.NewBackend(tmp)
// 	is.NoErr(err)
// 	dec := namaste.NewDeclaration("ocfl_object", "1.1")
// 	err = dec.Write(bak, path.Join("good-1", "b", "c"))
// 	is.NoErr(err)

// 	// test
// 	_, err = bak.StoreScan("bad-1")
// 	is.True(err != nil)
// 	_, err = bak.StoreScan("bad-2")
// 	is.True(err != nil)
// 	entries, err := bak.StoreScan("good-1")
// 	is.NoErr(err)
// 	is.Equal(entries["good-1/b/c"], dec.Name())
// }
