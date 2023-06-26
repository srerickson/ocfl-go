package ocfl_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/matryer/is"
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend/memfs"
)

func TestParseName(t *testing.T) {
	table := map[string]ocfl.Declaration{
		`0=ocfl_1.0`: {`ocfl`, ocfl.Spec{1, 0}},
		`0=oc_1.1`:   {`oc`, ocfl.Spec{1, 1}},
		`1=ocfl_1.0`: {``, ocfl.Spec{}},
		`0=AB_1`:     {``, ocfl.Spec{}},
	}
	for in, exp := range table {
		t.Run(in, func(t *testing.T) {
			is := is.New(t)
			var d ocfl.Declaration
			err := ocfl.ParseDeclaration(in, &d)
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
	err := ocfl.ValidateDeclaration(context.Background(), ocfl.NewFS(fsys), "0=hot_tub_12.1")
	is.NoErr(err)
	err = ocfl.ValidateDeclaration(context.Background(), ocfl.NewFS(fsys), "0=hot_bath_12.1")
	is.True(err != nil)
	err = ocfl.ValidateDeclaration(context.Background(), ocfl.NewFS(fsys), "1=hot_tub_12.1")
	is.True(err != nil)
}

func TestWriteDeclaration(t *testing.T) {
	is := is.New(t)
	fsys := memfs.New()
	ctx := context.Background()
	v := ocfl.Spec{12, 1}
	dec := &ocfl.Declaration{"ocfl", v}
	err := ocfl.WriteDeclaration(ctx, fsys, ".", *dec)
	is.NoErr(err)
	inf, err := fsys.ReadDir(ctx, ".")
	is.NoErr(err)
	out, err := ocfl.FindDeclaration(inf)
	is.NoErr(err)
	is.True(out.Type == "ocfl")
	is.True(out.Version == v)
	err = ocfl.ValidateDeclaration(ctx, fsys, dec.Name())
	is.NoErr(err)
}
