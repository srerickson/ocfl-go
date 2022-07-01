package namaste_test

import (
	"context"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"github.com/matryer/is"
	"github.com/srerickson/ocfl/backend/local"
	"github.com/srerickson/ocfl/namaste"
	spec "github.com/srerickson/ocfl/spec"
)

func TestParseName(t *testing.T) {
	table := map[string]namaste.Declaration{
		`0=ocfl_1.0`: {`ocfl`, spec.Num{1, 0}},
		`0=oc_1.1`:   {`oc`, spec.Num{1, 1}},
		`1=ocfl_1.0`: {``, spec.Num{}},
		`0=AB_1`:     {``, spec.Num{}},
	}
	for in, exp := range table {
		t.Run(in, func(t *testing.T) {
			is := is.New(t)
			var d namaste.Declaration
			err := namaste.ParseName(in, &d)
			if exp.Type != "" {
				is.NoErr(err)
				is.Equal(d, exp)
			} else {
				is.True(err != nil)
			}
		})
	}

}

func TestValidate(t *testing.T) {
	is := is.New(t)
	fsys := fstest.MapFS{
		"0=hot_tub_12.1": &fstest.MapFile{
			Data: []byte("hot_tub_12.1\n")},
		"0=hot_bath_12.1": &fstest.MapFile{
			Data: []byte("hot_tub_12.1")},
		"1=hot_tub_12.1": &fstest.MapFile{
			Data: []byte("hot_tub_12.1\n")},
	}
	err := namaste.Validate(context.Background(), fsys, "0=hot_tub_12.1")
	is.NoErr(err)
	err = namaste.Validate(context.Background(), fsys, "0=hot_bath_12.1")
	is.True(err != nil)
	err = namaste.Validate(context.Background(), fsys, "1=hot_tub_12.1")
	is.True(err != nil)
}

func TestWriteDeclaration(t *testing.T) {
	is := is.New(t)
	tmpDir, err := os.MkdirTemp("", "tmp-namaste-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	w, _ := local.NewBackend(tmpDir)
	r := os.DirFS(tmpDir)
	v := spec.Num{12, 1}
	dec := &namaste.Declaration{"ocfl", v}

	err = dec.Write(w, ".")
	is.NoErr(err)

	inf, err := fs.ReadDir(r, ".")
	is.NoErr(err)
	out, err := namaste.FindDelcaration(inf)
	is.NoErr(err)
	is.True(out.Type == "ocfl")
	is.True(out.Version == v)
	err = namaste.Validate(context.Background(), r, dec.Name())
	is.NoErr(err)
}
