package ocfl_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/local"
	"github.com/srerickson/ocfl-go/internal/testutil"
)

func TestObject_Example(t *testing.T) {
	ctx := context.Background()
	tmpFS, err := local.NewFS(t.TempDir())
	be.NilErr(t, err)

	// open new-object-01, which doesn't exist
	id := "new-object-01"
	obj, err := ocfl.NewObject(ctx, tmpFS, id, ocfl.ObjectWithID(id))
	be.NilErr(t, err)
	be.False(t, obj.Exists()) // the object doesn't exist yet
	be.Equal(t, id, obj.ID()) // its ID is set

	// create the first version from bytes
	stage := ocfl.NewStage(obj.NewUpdate())
	be.NilErr(t, stage.AddBytes("README.txt", []byte("this is a test file"), digest.MD5))
	be.NilErr(t, stage.Update.Finalize("first version", ocfl.User{Name: "Mx. Robot"}))
	v1Obj, err := stage.Update.Apply(ctx, obj.FS(), obj.Path(), stage.Content)
	be.NilErr(t, err)          // update worked
	be.True(t, v1Obj.Exists()) // the object was created
	be.False(t, obj.Exists())  // obj is unchanged

	// object has expected version information values
	be.Equal(t, "new-object-01", v1Obj.ID())
	sourceVersion := v1Obj.Version(1)
	be.Nonzero(t, sourceVersion)
	be.Nonzero(t, sourceVersion.Created())
	be.Equal(t, "first version", sourceVersion.Message())
	be.Equal(t, ocfl.User{Name: "Mx. Robot"}, *sourceVersion.User())
	be.Nonzero(t, sourceVersion.State().PathMap()["README.txt"])

	// update a new version and upgrade to OCFL v1.1
	stage = ocfl.NewStage(v1Obj.NewUpdate())
	be.NilErr(t, stage.AddBytes("README.txt", []byte("this is a test file (v2)"), digest.MD5))
	be.NilErr(t, stage.AddBytes("new-data.csv", []byte("1,2,3"), digest.MD5))
	be.NilErr(t, stage.AddBytes("docs/note.txt", []byte("this is a note"), digest.MD5))
	be.NilErr(t, stage.Update.Finalize("second version", ocfl.User{Name: "Dr. Robot"},
		ocfl.UpdateWithOCFLSpec(ocfl.Spec1_1)))
	v2Obj, err := stage.Update.Apply(ctx, obj.FS(), obj.Path(), stage.Content)
	be.NilErr(t, err)
	be.Equal(t, "new-object-01", v2Obj.ID())
	be.Equal(t, ocfl.Spec1_1, v2Obj.Spec())
	be.Nonzero(t, v2Obj.Version(2).State().PathMap()["new-data.csv"])
	be.DeepEqual(t, []string{"md5"}, v2Obj.FixityAlgorithms())

	// check that the object is valid
	be.NilErr(t, ocfl.ValidateObject(ctx, v2Obj.FS(), v2Obj.Path()).Err())

	// create a logical FS of the version state
	logicalFS, err := v2Obj.VersionFS(ctx, 0)
	be.NilErr(t, err)

	// we can list files in a directory
	entries, err := fs.ReadDir(logicalFS, "docs")
	be.NilErr(t, err)
	be.Equal(t, 1, len(entries))

	// we can read files from the logical FS
	gotBytes, err := fs.ReadFile(logicalFS, "new-data.csv")
	be.NilErr(t, err)
	be.Equal(t, "1,2,3", string(gotBytes))

	// create a new object by forking head version of new-object-01: the
	// content comes from the source object.
	forkID := "new-object-02"
	sourceVersion = v2Obj.Version(0)
	be.Nonzero(t, sourceVersion)
	forkUpdate, err := ocfl.NewUpdate(ctx, tmpFS, forkID, forkID)
	be.NilErr(t, err)
	forkContent := &ocfl.ContentMap{}
	manifest := v2Obj.Manifest()
	for name, dig := range sourceVersion.State().Paths() {
		be.NilErr(t, forkUpdate.Add(name, dig, nil))
		forkContent.AddFile(dig, v2Obj.FS(), path.Join(v2Obj.Path(), manifest[dig][0]))
	}
	be.NilErr(t, forkUpdate.Finalize(sourceVersion.Message(), *sourceVersion.User()))
	forkObj, err := forkUpdate.Apply(ctx, tmpFS, forkID, forkContent)
	be.NilErr(t, err)
	be.NilErr(t, ocfl.ValidateObject(ctx, forkObj.FS(), forkObj.Path()).Err())
	be.True(t, sourceVersion.State().Eq(forkObj.Version(0).State()))
}

func TestNewObject(t *testing.T) {
	ctx := context.Background()
	fsys := ocflfs.DirFS(objectFixturesPath)
	type testCase struct {
		fs     ocflfs.FS
		path   string
		opts   []ocfl.ObjectOption
		expect func(*testing.T, *ocfl.Object, error)
	}
	v1Inventory, err := ocfl.ReadInventory(ctx, fsys, "1.0/good-objects/spec-ex-full/v1")
	be.NilErr(t, err)
	testCases := map[string]testCase{
		"ok 1.0": {
			fs:   fsys,
			path: "1.0/good-objects/spec-ex-full",
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
		"ok 1.1": {
			fs:   fsys,
			path: "1.1/good-objects/spec-ex-full",
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
		"not existin, no id": {
			fs:   fsys,
			path: "new-dir",
			expect: func(t *testing.T, obj *ocfl.Object, err error) {
				be.Nonzero(t, err) // ID is required
			},
		},
		"not existing, with id": {
			fs:   fsys,
			path: "new-dir",
			opts: []ocfl.ObjectOption{ocfl.ObjectWithID("new-object")},
			expect: func(t *testing.T, obj *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
		"not existing, must exist": {
			fs:   fsys,
			path: "missing-dir",
			opts: []ocfl.ObjectOption{ocfl.ObjectMustExist()},
			expect: func(t *testing.T, obj *ocfl.Object, err error) {
				be.True(t, errors.Is(err, fs.ErrNotExist))
			},
		},
		"with skip inventory sidecar validation": {
			fs:   fsys,
			path: "1.1/bad-objects/E060_E064_root_inventory_digest_mismatch",
			opts: []ocfl.ObjectOption{
				ocfl.ObjectSkipRootSidecarValidation(),
			},
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
		"sidecar validation error": {
			fs:   fsys,
			path: "1.1/bad-objects/E060_E064_root_inventory_digest_mismatch",
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				// error from failed sidecar validation
				var expectErr *digest.DigestError
				be.True(t, errors.As(err, &expectErr))
			},
		},
		"non-root inventory sidecar validation error": {
			fs:   fsys,
			path: "1.1/good-objects/spec-ex-full",
			opts: []ocfl.ObjectOption{
				ocfl.ObjectWithInventory(v1Inventory),
			},
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				// the given inventory failes validation with root sidecar
				be.Nonzero(t, err)
			},
		},
		"with non-root inventory skip validation": {
			fs:   fsys,
			path: "1.1/good-objects/spec-ex-full",
			opts: []ocfl.ObjectOption{
				ocfl.ObjectWithInventory(v1Inventory),
				ocfl.ObjectSkipRootSidecarValidation(),
			},
			expect: func(t *testing.T, obj *ocfl.Object, err error) {
				be.NilErr(t, err)
				// object was loaded with version inventory
				be.Equal(t, ocfl.V(1), obj.Head())
			},
		},
		"missing ID": {
			fs:   fsys,
			path: "1.1/bad-objects/E003_E063_empty",
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.Nonzero(t, err)
				be.True(t, errors.Is(err, ocfl.ErrNoObjectID))
			},
		},
		"non-comforming contents": {
			fs:   fsys,
			path: "1.1/bad-objects/E003_E063_empty",
			opts: []ocfl.ObjectOption{
				ocfl.ObjectWithID("new"),
			},
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.Nonzero(t, err)
				be.In(t, "non-conforming contents", err.Error())
			},
		},
	}
	for name, tCase := range testCases {
		t.Run(name, func(t *testing.T) {
			obj, err := ocfl.NewObject(ctx, tCase.fs, tCase.path, tCase.opts...)
			tCase.expect(t, obj, err)
		})
	}
}

func TestObject_VersionFS(t *testing.T) {
	ctx := context.Background()
	fixturesDir := filepath.Join(`testdata`, `object-fixtures`, `1.1`, `good-objects`)
	fsys := ocflfs.DirFS(fixturesDir)
	fixtures := []string{"minimal_no_content", "updates_all_actions"}
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			obj, err := ocfl.NewObject(ctx, fsys, fixture)
			be.NilErr(t, err)
			for _, vnum := range obj.Head().Lineage() {
				t.Run(vnum.String(), func(t *testing.T) {
					ver := obj.Version(vnum.Num())
					logicalFS, err := obj.VersionFS(ctx, vnum.Num())
					be.NilErr(t, err)
					err = fstest.TestFS(logicalFS, ver.State().AllPaths()...)
					be.NilErr(t, err)
				})
			}
		})
	}
}

func TestObject_NewUpdate(t *testing.T) {
	ctx := context.Background()
	fixture := filepath.Join(objectFixturesPath, `1.1`, `good-objects`, `spec-ex-full`)
	t.Run("existing object", func(t *testing.T) {
		obj, err := ocfl.NewObject(ctx, ocflfs.DirFS(fixture), ".")
		be.NilErr(t, err)
		u := obj.NewUpdate()
		be.Equal(t, obj.ID(), u.ID())
		be.Equal(t, obj.DigestAlgorithm().ID(), u.DigestAlgorithm().ID())
		be.Equal(t, obj.InventoryDigest(), u.BaseInventoryDigest())
		be.Equal(t, ocfl.V(4), u.NextHead())
		be.True(t, obj.Version(0).State().Eq(u.State()))
		be.False(t, u.Finalized())
	})
	t.Run("new object", func(t *testing.T) {
		obj, err := ocfl.NewObject(ctx, ocflfs.DirFS(t.TempDir()), "obj", ocfl.ObjectWithID("obj-1"))
		be.NilErr(t, err)
		u := obj.NewUpdate()
		be.Equal(t, "obj-1", u.ID())
		be.Equal(t, digest.SHA512.ID(), u.DigestAlgorithm().ID())
		be.Zero(t, u.BaseInventoryDigest())
		be.Equal(t, ocfl.V(1), u.NextHead())
		be.Equal(t, 0, len(u.State()))
	})
	// Changing the update's state must not change the object's version
	// states: an aliased state would rewrite earlier versions in the next
	// inventory.
	t.Run("update state is a copy", func(t *testing.T) {
		fsys := testutil.TmpLocalFS(t, fixture)
		obj, err := ocfl.NewObject(ctx, fsys, "spec-ex-full")
		be.NilErr(t, err)
		headState := obj.Version(0).State()
		stage := ocfl.NewStage(obj.NewUpdate())
		be.NilErr(t, stage.Rename("foo/bar.xml", "baz.xml"))
		be.NilErr(t, stage.Remove("image.tiff"))
		be.True(t, headState.Eq(obj.Version(0).State()))
		newObj := commit(t, obj, stage, "v4")
		be.True(t, headState.Eq(newObj.Version(3).State()))
		be.True(t, headState.Eq(obj.Version(0).State()))
		be.DeepEqual(t, []string{"baz.xml", "empty2.txt"}, newObj.Version(0).State().AllPaths())
		be.NilErr(t, ocfl.ValidateObject(ctx, fsys, newObj.Path()).Err())
	})
}

func TestValidateObject(t *testing.T) {
	ctx := context.Background()
	fixturePath := filepath.Join(`testdata`, `object-fixtures`, `1.1`)
	fsys := ocflfs.DirFS(filepath.Join(fixturePath, `bad-objects`))
	t.Run("skip digests", func(t *testing.T) {
		// object reports no validation if digests aren't checked
		objPath := `E093_fixity_digest_mismatch`
		v := ocfl.ValidateObject(ctx, fsys, objPath, ocfl.ValidationSkipDigest())
		be.NilErr(t, v.Err())
	})
}

func TestValidateObject_Fixtures(t *testing.T) {
	ctx := context.Background()
	for _, spec := range []string{`1.0`, `1.1`} {
		t.Run(spec, func(t *testing.T) {
			fixturePath := filepath.Join(`testdata`, `object-fixtures`, spec)
			goodObjPath := filepath.Join(fixturePath, `good-objects`)
			badObjPath := filepath.Join(fixturePath, `bad-objects`)
			warnObjPath := filepath.Join(fixturePath, `warn-objects`)
			t.Run("Valid objects", func(t *testing.T) {
				fsys := ocflfs.NewWrapFS(os.DirFS(goodObjPath))
				goodObjects, err := ocflfs.ReadDir(context.Background(), fsys, ".")
				be.NilErr(t, err)
				for _, dir := range goodObjects {
					t.Run(dir.Name(), func(t *testing.T) {
						result := ocfl.ValidateObject(ctx, fsys, dir.Name())
						be.NilErr(t, result.Err())
						be.NilErr(t, result.WarnErr())
					})
				}
			})
			t.Run("Invalid objects", func(t *testing.T) {
				fsys := ocflfs.NewWrapFS(os.DirFS(badObjPath))
				badObjects, err := ocflfs.ReadDir(context.Background(), fsys, ".")
				be.NilErr(t, err)
				for _, dir := range badObjects {
					if !dir.IsDir() {
						continue
					}
					t.Run(dir.Name(), func(t *testing.T) {
						result := ocfl.ValidateObject(ctx, fsys, dir.Name())
						be.True(t, result.Err() != nil)
						expectFixtureErrors(t, dir.Name(), result.Errors()...)
					})
				}
			})
			t.Run("Warning objects", func(t *testing.T) {
				fsys := ocflfs.NewWrapFS(os.DirFS(warnObjPath))
				warnObjects, err := ocflfs.ReadDir(context.Background(), fsys, ".")
				be.NilErr(t, err)
				for _, dir := range warnObjects {
					t.Run(dir.Name(), func(t *testing.T) {
						result := ocfl.ValidateObject(ctx, fsys, dir.Name())
						be.NilErr(t, result.Err())
						t.Log(result.WarnErr())
						be.True(t, len(result.WarnErrors()) > 0)
					})
				}
			})
		})
	}

}

// check that errs includes code expected from the fixture name
func expectFixtureErrors(t *testing.T, fixtureName string, errs ...error) {
	t.Helper()
	// if these codes are expected by the fixture but missing from errs, the test won't fail.
	// E001 for invalid_version_format fixtures needs more investigation
	dontFaileCodes := []string{"E001"}
	codeRegexp := regexp.MustCompile(`^E\d{3}$`)
	expCodes := map[string]bool{}
	gotCodes := map[string]bool{}
	for part := range strings.SplitSeq(fixtureName, "_") {
		if codeRegexp.MatchString(part) {
			expCodes[part] = true
		}
	}
	var gotExpected bool
	for _, e := range errs {
		var vErr *ocfl.ValidationError
		if errors.As(e, &vErr) {
			c := vErr.ValidationCode.Code
			gotCodes[c] = true
			if expCodes[c] {
				gotExpected = true
			}
		}
	}
	expKeys := slices.Collect(maps.Keys(expCodes))
	gotKeys := slices.Collect(maps.Keys(gotCodes))
	sort.Strings(expKeys)
	sort.Strings(gotKeys)
	if len(gotKeys) == 0 {
		gotKeys = append(gotKeys, "[none]")
	}
	var desc string
	if !gotExpected {
		got := strings.Join(gotKeys, ", ")
		exp := strings.Join(expKeys, ", ")
		desc = fmt.Sprintf("didn't get expected error code: got %s, expected %s", got, exp)
	}
	if !gotExpected {
		// if all the expected codes are in dontFailCodes, log but don't fail
		if isSubset(expKeys, dontFaileCodes) {
			t.Log(fixtureName+":", desc)
			return
		}
		t.Error(fixtureName+":", desc)
	}

}

func TempDirFixtureCopy(t *testing.T, fixture string) string {
	t.Helper()
	tmpDir := t.TempDir()
	if err := os.CopyFS(tmpDir, os.DirFS(fixture)); err != nil {
		t.Error(err)
	}
	return tmpDir
}

// return if a is subset of b
func isSubset(a, b []string) bool {
	for _, aVal := range a {
		if !slices.Contains(b, aVal) {
			return false
		}
	}
	return true
}
