package ocfl_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/matryer/is"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/memfs"
)

func TestParseName(t *testing.T) {
	table := map[string]ocfl.Namaste{
		`0=ocfl_1.0`: {`ocfl`, ocfl.Spec1_0},
		`0=oc_1.1`:   {`oc`, ocfl.Spec1_1},
		`1=ocfl_1.0`: {``, ocfl.Spec("")},
		`0=AB_1`:     {``, ocfl.Spec("")},
	}
	for in, exp := range table {
		t.Run(in, func(t *testing.T) {
			is := is.New(t)
			d, err := ocfl.ParseNamaste(in)
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
	err := ocfl.ValidateNamaste(context.Background(), ocfl.NewFS(fsys), "0=hot_tub_12.1")
	is.NoErr(err)
	err = ocfl.ValidateNamaste(context.Background(), ocfl.NewFS(fsys), "0=hot_bath_12.1")
	is.True(err != nil)
	err = ocfl.ValidateNamaste(context.Background(), ocfl.NewFS(fsys), "1=hot_tub_12.1")
	is.True(err != nil)
}

func TestWriteDeclaration(t *testing.T) {
	is := is.New(t)
	fsys := memfs.New()
	ctx := context.Background()
	v := ocfl.Spec("12.1")
	dec := &ocfl.Namaste{"ocfl", v}
	err := ocfl.WriteDeclaration(ctx, fsys, ".", *dec)
	is.NoErr(err)
	inf, err := fsys.ReadDir(ctx, ".")
	is.NoErr(err)
	out, err := ocfl.FindNamaste(inf)
	is.NoErr(err)
	is.True(out.Type == "ocfl")
	is.True(out.Version == v)
	err = ocfl.ValidateNamaste(ctx, fsys, dec.Name())
	is.NoErr(err)
}
