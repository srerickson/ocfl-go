package ocfl_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/memfs"
)

func TestSpecCmp(t *testing.T) {
	type testCase struct {
		v1         ocfl.Spec
		v2         ocfl.Spec
		wantResult int
		wantPanic  bool
	}
	testCases := []testCase{
		{v1: ocfl.Spec1_1, v2: ocfl.Spec1_0, wantResult: 1},
		{v1: ocfl.Spec1_0, v2: ocfl.Spec1_1, wantResult: -1},
		{v1: ocfl.Spec1_1, v2: ocfl.Spec1_1, wantResult: 0},
		{v1: ocfl.Spec("2.0"), v2: ocfl.Spec1_1, wantResult: 1},
		{v1: ocfl.Spec("1.1"), v2: ocfl.Spec("1.1-draft"), wantResult: 1},
		{v1: ocfl.Spec("2.2-draft"), v2: ocfl.Spec("2.2"), wantResult: -1},
		{v1: ocfl.Spec("1.1-draft"), v2: ocfl.Spec("1.1-new"), wantResult: 0},
		{v1: ocfl.Spec(""), v2: ocfl.Spec1_0, wantPanic: true},
		{v1: ocfl.Spec1_0, v2: ocfl.Spec("1"), wantPanic: true},
	}
	for i, tcase := range testCases {
		t.Run(fmt.Sprintf("case%d--%v vs %v", i, tcase.v1, tcase.v2), func(t *testing.T) {
			defer func() {
				be.Equal(t, tcase.wantPanic, recover() != nil)
			}()
			be.Equal(t, tcase.wantResult, tcase.v1.Cmp(tcase.v2))
		})
	}
}

func TestSpecValid(t *testing.T) {
	type testCase struct {
		val     ocfl.Spec
		isValid bool
	}
	testCases := []testCase{
		// valid
		{val: ocfl.Spec1_1, isValid: true},
		{val: ocfl.Spec1_0, isValid: true},
		{val: ocfl.Spec("2.3"), isValid: true},
		{val: ocfl.Spec("1.55-test"), isValid: true},
		// invalid
		{val: ocfl.Spec(""), isValid: false},
		{val: ocfl.Spec("1"), isValid: false},
		{val: ocfl.Spec("1.-fun"), isValid: false},
		{val: ocfl.Spec("a.12"), isValid: false},
		{val: ocfl.Spec("."), isValid: false},
		{val: ocfl.Spec("1.b"), isValid: false},
		{val: ocfl.Spec("1-"), isValid: false},
	}
	for i, tcase := range testCases {
		t.Run(fmt.Sprintf("case%d--%v", i, tcase.val), func(t *testing.T) {
			be.Equal(t, tcase.isValid, tcase.val.Valid() == nil)
		})
	}
}

func TestSpecEmpty(t *testing.T) {
	be.True(t, ocfl.Spec("").Empty())
	be.False(t, ocfl.Spec1_0.Empty())
}

func TestParseInventoryType(t *testing.T) {
	type testCase struct {
		in    string
		out   ocfl.Spec
		valid bool
	}
	testCases := []testCase{
		// valid
		{"https://ocfl.io/1.0/spec/#inventory", ocfl.Spec1_0, true},
		{"https://ocfl.io/1.1/spec/#inventory", ocfl.Spec1_1, true},
		{"https://ocfl.io/2.0-draft/spec/#inventory", ocfl.Spec("2.0-draft"), true},
		// invalid
		{"https://ocfl.io/./spec/#inventory", ocfl.Spec(""), false},
		{"https://ocfl.io/spec/#inventory", ocfl.Spec(""), false},
	}
	for i, tcase := range testCases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			inv := ocfl.InvType{}
			err := inv.UnmarshalText([]byte(tcase.in))
			if tcase.valid {
				be.NilErr(t, err)
			} else {
				be.True(t, err != nil)
			}
			be.Equal(t, inv.Spec, tcase.out)
		})
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
