package ocfl_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/local"
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
			d, err := ocfl.ParseNamaste(in)
			if exp.Type != "" {
				be.NilErr(t, err)
				be.Equal(t, exp, d)
			} else {
				be.True(t, err != nil)
			}
		})
	}

}

func TestValidate(t *testing.T) {
	fsys := fstest.MapFS{
		"0=hot_tub_12.1": &fstest.MapFile{
			Data: []byte("hot_tub_12.1\n")},
		"0=hot_bath_12.1": &fstest.MapFile{
			Data: []byte("hot_tub_12.1")},
		"1=hot_tub_12.1": &fstest.MapFile{
			Data: []byte("hot_tub_12.1\n")},
	}
	err := ocfl.ValidateNamaste(context.Background(), ocflfs.NewFS(fsys), "0=hot_tub_12.1")
	be.NilErr(t, err)
	err = ocfl.ValidateNamaste(context.Background(), ocflfs.NewFS(fsys), "0=hot_bath_12.1")
	be.True(t, err != nil)
	err = ocfl.ValidateNamaste(context.Background(), ocflfs.NewFS(fsys), "1=hot_tub_12.1")
	be.True(t, err != nil)
}

func TestWriteDeclaration(t *testing.T) {
	ctx := context.Background()
	fsys, err := local.NewFS(t.TempDir())
	be.NilErr(t, err)
	v := ocfl.Spec("12.1")
	dec := &ocfl.Namaste{"ocfl", v}
	be.NilErr(t, ocfl.WriteDeclaration(ctx, fsys, ".", *dec))
	inf, err := ocflfs.ReadDir(ctx, fsys, ".")
	be.NilErr(t, err)
	out, err := ocfl.FindNamaste(inf)
	be.NilErr(t, err)
	be.Equal(t, "ocfl", out.Type)
	be.Equal(t, v, out.Version)
	be.NilErr(t, ocfl.ValidateNamaste(ctx, fsys, dec.Name()))
}
