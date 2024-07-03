package ocfl_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/local"
	_ "github.com/srerickson/ocfl-go/ocflv1"
)

func TestObject(t *testing.T) {
	t.Run("Example", testObjectExample)
	t.Run("Open", testOpenObject)
}

func testObjectExample(t *testing.T) {
	ocflv1_0 := ocfl.MustGetOCFL(ocfl.Spec1_0)
	// ocflv1_1 := ocfl.MustGetOCFL(ocfl.Spec1_1)
	ctx := context.Background()
	tmpFS, err := local.NewFS(t.TempDir())
	be.NilErr(t, err)

	// open new object in ocfl v1.0 mode
	obj, err := ocfl.OpenObject(ctx, tmpFS, "new-object-01", ocfl.ObjectUseOCFL(ocflv1_0))
	be.NilErr(t, err)

	be.False(t, obj.Exists())                  // the object doesn't exist yet
	be.Equal(t, ocfl.Spec1_0, obj.UsingSpec()) // the object was opened in ocfl v1.0 mode.

	// commit new object version from bytes:
	v1Content := map[string][]byte{
		"README.txt": []byte("this is a test file"),
	}
	stage, err := ocfl.StageBytes(v1Content, ocfl.SHA512, ocfl.MD5)
	be.NilErr(t, err)
	err = obj.Commit(ctx, &ocfl.Commit{
		ID:      "new-object-01",
		Stage:   stage,
		User:    ocfl.User{Name: "Mx. Robot"},
		Message: "first version",
	})
	be.NilErr(t, err)        // commit worked
	be.True(t, obj.Exists()) // the object was created

	// object has expected inventory values
	be.Equal(t, "new-object-01", obj.Inventory().ID())
	be.Nonzero(t, obj.Inventory().Version(0).State().PathMap()["README.txt"])

	// commit a new version and upgrade to OCFL v1.1
	v2Content := map[string][]byte{
		"README.txt":   []byte("this is a test file (v2)"),
		"new-data.csv": []byte("1,2,3"),
	}
	stage, err = ocfl.StageBytes(v2Content, ocfl.SHA512, ocfl.MD5)
	be.NilErr(t, err)
	err = obj.Commit(ctx, &ocfl.Commit{
		ID:      "new-object-01",
		Stage:   stage,
		User:    ocfl.User{Name: "Dr. Robot"},
		Message: "second version",
		Upgrade: ocfl.Spec1_1,
	})
	be.NilErr(t, err)
	be.Equal(t, ocfl.Spec1_1, obj.UsingSpec())
	be.Equal(t, ocfl.Spec1_1, obj.Inventory().Spec())
	be.Nonzero(t, obj.Inventory().Version(2).State().PathMap()["new-data.csv"])

	// Other things to do:
	// create another new object that forks new-object-01
	// roll-back an object to a previous version
	// interact with an object's extensions
}

// OpenObject unit tests
func testOpenObject(t *testing.T) {
	ctx := context.Background()
	fsys := ocfl.DirFS(objectFixturesPath)
	// ocflv1_1 := ocfl.MustGetOCFL(ocfl.Spec1_1)
	// expectErrIs := func(t *testing.T, err error, wantErr error) {
	// 	t.Helper()
	// 	if !errors.Is(err, wantErr) {
	// 		t.Errorf("wanted error: %q; got error: %q", wantErr, err)
	// 	}
	// }

	type testCase struct {
		ctx    context.Context
		fs     ocfl.FS
		path   string
		opts   []func(*ocfl.Object)
		expect func(*testing.T, *ocfl.Object, error)
	}
	testCases := map[string]testCase{
		"ok 1.0": {
			fs:   fsys,
			path: "1.0/good-objects/spec-ex-full",
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
		// FIXME
		// "wrong spec 1.0": {
		// 	fs:   fsys,
		// 	path: "1.0/good-objects/spec-ex-full",
		// 	opts: []func(*ocfl.Object){ocfl.ObjectUseOCFL(ocflv1_1)},
		// 	expect: func(t *testing.T, _ *ocfl.Object, err error) {
		// 		expectErrIs(t, err, ocfl.ErrObjectNamasteNotExist)
		// 	},
		// },
		"ok 1.1": {
			fs:   fsys,
			path: "1.1/good-objects/spec-ex-full",
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
		"not existing": {
			ctx:  ctx,
			fs:   fsys,
			path: "new-dir",
			expect: func(t *testing.T, obj *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
		"empty": {
			ctx:  ctx,
			fs:   fsys,
			path: "1.1/bad-objects/E003_E063_empty",
			opts: []func(*ocfl.Object){ocfl.ObjectUseOCFL(ocfl.MustGetOCFL(ocfl.Spec1_1))},
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
			obj, err := ocfl.OpenObject(tCase.ctx, tCase.fs, tCase.path, tCase.opts...)
			tCase.expect(t, obj, err)
		})
		i++
	}
}
