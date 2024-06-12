package ocfl_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	_ "github.com/srerickson/ocfl-go/ocflv1"
)

func TestOpenObject(t *testing.T) {
	ctx := context.Background()
	fsys := ocfl.DirFS(objectFixturesPath)

	expectNilErr := func(t *testing.T, _ ocfl.Object, err error) {
		be.NilErr(t, err)
	}
	expectErrIs := func(wantErr error) func(t *testing.T, _ ocfl.Object, err error) {
		return func(t *testing.T, _ ocfl.Object, err error) {
			be.True(t, errors.Is(err, wantErr))
		}
	}

	type testCase struct {
		ctx    context.Context
		root   *ocfl.ObjectRoot
		opts   *ocfl.ObjectOptions
		expect func(*testing.T, ocfl.Object, error)
	}
	testCases := map[string]testCase{
		"ok 1.0": {
			root: &ocfl.ObjectRoot{FS: fsys, Path: "1.0/good-objects/spec-ex-full"},
		},
		"ok 1.1": {
			root: &ocfl.ObjectRoot{FS: fsys, Path: "1.1/good-objects/spec-ex-full"},
		},
		"missing": {
			ctx:    ctx,
			root:   &ocfl.ObjectRoot{FS: fsys, Path: "missing"},
			expect: expectErrIs(fs.ErrNotExist),
		},
		"empty": {
			ctx:    ctx,
			root:   &ocfl.ObjectRoot{FS: fsys, Path: "1.1/bad-objects/E003_E063_empty"},
			expect: expectErrIs(ocfl.ErrObjectNamasteNotExist),
		},
	}
	i := 0
	for name, tCase := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, name), func(t *testing.T) {
			if tCase.ctx == nil {
				tCase.ctx = ctx
			}
			if tCase.expect == nil {
				tCase.expect = expectNilErr
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
