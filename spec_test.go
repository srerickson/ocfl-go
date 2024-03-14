package ocfl_test

import (
	"context"
	"testing"

	"github.com/matryer/is"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/memfs"
)

type invTypeTest struct {
	in    string
	out   ocfl.Spec
	valid bool
}

var invTypeTable = []invTypeTest{
	// valid
	{"https://ocfl.io/1.0/spec/#inventory", ocfl.Spec1_0, true},
	{"https://ocfl.io/1.1/spec/#inventory", ocfl.Spec1_1, true},
	// invalid
	{"https://ocfl.io/1/spec/#inventory", ocfl.Spec(""), false},
	{"https://ocfl.io/spec/#inventory", ocfl.Spec(""), false},
}

func TestParseInventoryType(t *testing.T) {
	is := is.New(t)
	for _, t := range invTypeTable {
		inv := ocfl.InvType{}
		err := inv.UnmarshalText([]byte(t.in))
		if t.valid {
			is.NoErr(err)
		} else {
			is.True(err != nil)
		}
		is.Equal(inv.Spec, t.out)
	}
}

func TestWriteSpecFile(t *testing.T) {
	ctx := context.Background()
	fsys := memfs.New()
	test := func(spec ocfl.Spec) {
		name, err := ocfl.WriteSpecFile(ctx, fsys, "dir1", spec)
		if err != nil {
			t.Fatal(err)
		}
		f, err := fsys.OpenFile(ctx, name)
		if err != nil {
			t.Fatalf("file doesn't exist: %s", name)
		}
		defer f.Close()
		// again
		_, err = ocfl.WriteSpecFile(ctx, fsys, "dir1", spec)
		if err == nil {
			t.Fatal("expected an error")
		}
	}
	test(ocfl.Spec1_0)
	test(ocfl.Spec1_1)
	// expect an error
	_, err := ocfl.WriteSpecFile(ctx, fsys, "dir1", ocfl.Spec("3.0"))
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestSpecEmpty(t *testing.T) {
	if !(ocfl.Spec("")).Empty() {
		t.Error("empty spec value should be Empty()")
	}
	if (ocfl.Spec1_0).Empty() {
		t.Error("non-empty spec value should not be Empty()")
	}
}
