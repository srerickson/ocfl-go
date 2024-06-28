package ocfl_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	_ "github.com/srerickson/ocfl-go/ocflv1"
)

func TestOpenObject(t *testing.T) {
	ctx := context.Background()
	fsys := ocfl.DirFS(objectFixturesPath)

	expectErrIs := func(t *testing.T, err error, wantErr error) {
		t.Helper()
		if !errors.Is(err, wantErr) {
			t.Errorf("wanted error: %q; got error: %q", wantErr, err)
		}
	}

	type testCase struct {
		ctx    context.Context
		root   *ocfl.ObjectRoot
		opts   *ocfl.ObjectOptions
		expect func(*testing.T, *ocfl.Object, error)
	}
	testCases := map[string]testCase{
		"ok 1.0": {
			root: &ocfl.ObjectRoot{FS: fsys, Path: "1.0/good-objects/spec-ex-full"},
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
		"wrong spec 1.0": {
			root: &ocfl.ObjectRoot{FS: fsys, Path: "1.0/good-objects/spec-ex-full"},
			opts: &ocfl.ObjectOptions{OCFL: ocfl.MustGetOCFL(ocfl.Spec1_1)},
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				expectErrIs(t, err, ocfl.ErrObjectNamasteNotExist)
			},
		},
		"ok 1.1": {
			root: &ocfl.ObjectRoot{FS: fsys, Path: "1.1/good-objects/spec-ex-full"},
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
		"not existing": {
			ctx:  ctx,
			root: &ocfl.ObjectRoot{FS: fsys, Path: "new-dir"},
			expect: func(t *testing.T, obj *ocfl.Object, err error) {
				be.NilErr(t, err)
				exists, err := obj.Exists(ctx)
				be.NilErr(t, err)
				be.False(t, exists)
			},
		},
		"empty": {
			ctx:  ctx,
			root: &ocfl.ObjectRoot{FS: fsys, Path: "1.1/bad-objects/E003_E063_empty"},
			opts: &ocfl.ObjectOptions{OCFL: ocfl.MustGetOCFL(ocfl.Spec1_1)},
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
	}
	i := 0
	for name, tCase := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, name), func(t *testing.T) {
			if tCase.ctx == nil {
				tCase.ctx = ctx
			}
			obj, err := ocfl.OpenObject(tCase.ctx, tCase.root, func(opt *ocfl.ObjectOptions) {
				if tCase.opts != nil {
					*opt = *tCase.opts
				}
			})
			tCase.expect(t, obj, err)
		})
		i++
	}
}
