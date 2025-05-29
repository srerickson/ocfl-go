package ocfl_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/fs/local"
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
	be.Equal(t, id, obj.ID()) // its ID isn't set

	// update new object version from bytes:
	v1Content := map[string][]byte{
		"README.txt": []byte("this is a test file"),
	}
	stage, err := ocfl.StageBytes(v1Content, digest.SHA512, digest.MD5)
	be.NilErr(t, err)
	_, err = obj.Update(
		ctx,
		stage,
		"first version",
		ocfl.User{Name: "Mx. Robot"},
	)
	be.NilErr(t, err)        // update worked
	be.True(t, obj.Exists()) // the object was created

	// object has expected inventory values
	be.Equal(t, "new-object-01", obj.ID())
	sourceVersion := obj.Version(1)
	be.Nonzero(t, sourceVersion)
	be.Nonzero(t, sourceVersion.State().PathMap()["README.txt"])

	// update a new version and upgrade to OCFL v1.1
	v2Content := map[string][]byte{
		"README.txt":    []byte("this is a test file (v2)"),
		"new-data.csv":  []byte("1,2,3"),
		"docs/note.txt": []byte("this is a note"),
	}
	stage, err = ocfl.StageBytes(v2Content, digest.SHA512, digest.MD5)
	be.NilErr(t, err)
	_, err = obj.Update(
		ctx,
		stage,
		"second version",
		ocfl.User{Name: "Dr. Robot"},
		ocfl.UpdateWithOCFLSpec(ocfl.Spec1_1),
	)
	be.NilErr(t, err)
	be.Equal(t, "new-object-01", obj.ID())
	be.Equal(t, ocfl.Spec1_1, obj.Spec())
	be.Nonzero(t, obj.Version(2).State().PathMap()["new-data.csv"])
	be.DeepEqual(t, []string{"md5"}, obj.FixityAlgorithms())

	// check that the object is valid
	be.NilErr(t, ocfl.ValidateObject(ctx, obj.FS(), obj.Path()).Err())
	// be.NilErr(t, result.Warning)

	// create a logical FS of the version state
	logicalFS, err := obj.VersionFS(ctx, 0)
	be.NilErr(t, err)

	// we can list files in a directory
	entries, err := fs.ReadDir(logicalFS, "docs")
	be.NilErr(t, err)
	be.Equal(t, 1, len(entries))

	// we can read files from the logical FS
	gotBytes, err := fs.ReadFile(logicalFS, "new-data.csv")
	be.NilErr(t, err)
	be.Equal(t, "1,2,3", string(gotBytes))

	// create a new object by forking head version of new-object-01
	forkID := "new-object-02"
	sourceVersion = obj.Version(0)
	be.Nonzero(t, sourceVersion)
	sourceStage := obj.VersionStage(0)
	be.Nonzero(t, sourceStage)
	forkObj, err := ocfl.NewObject(ctx, tmpFS, forkID, ocfl.ObjectWithID(forkID))
	be.NilErr(t, err)
	_, err = forkObj.Update(
		ctx,
		sourceStage,
		sourceVersion.Message(),
		*sourceVersion.User(),
	)
	be.NilErr(t, err)
	be.NilErr(t, ocfl.ValidateObject(ctx, forkObj.FS(), forkObj.Path()).Err())
	be.True(t, sourceVersion.State().Eq(forkObj.Version(0).State()))
}

// OpenObject unit tests
func TestNewObject(t *testing.T) {
	ctx := context.Background()
	fsys := ocflfs.DirFS(objectFixturesPath)
	type testCase struct {
		fs     ocflfs.FS
		path   string
		opts   []ocfl.ObjectOption
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
		"ok 1.1": {
			fs:   fsys,
			path: "1.1/good-objects/spec-ex-full",
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
		"not existing": {
			fs:   fsys,
			path: "new-dir",
			expect: func(t *testing.T, obj *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
		"not existing, must exist": {
			fs:   fsys,
			path: "new-dir",
			opts: []ocfl.ObjectOption{ocfl.ObjectMustExist()},
			expect: func(t *testing.T, obj *ocfl.Object, err error) {
				be.True(t, errors.Is(err, fs.ErrNotExist))
			},
		},
		"with skip inventory sidecar validation": {
			fs:   fsys,
			path: "1.1/bad-objects/E060_E064_root_inventory_digest_mismatch",
			opts: []ocfl.ObjectOption{
				ocfl.ObjectSkipSidecarValidation(),
			},
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
		"without skip inventory sidecar validation": {
			fs:   fsys,
			path: "1.1/bad-objects/E060_E064_root_inventory_digest_mismatch",
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				var expectErr *digest.DigestError
				be.True(t, errors.As(err, &expectErr))
			},
		},
		"empty": {
			fs:   fsys,
			path: "1.1/bad-objects/E003_E063_empty",
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.Nonzero(t, err)
			},
		},
	}
	i := 0
	for name, tCase := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, name), func(t *testing.T) {
			obj, err := ocfl.NewObject(ctx, tCase.fs, tCase.path, tCase.opts...)
			tCase.expect(t, obj, err)
		})
		i++
	}
}

func TestObject_Update(t *testing.T) {
	ctx := context.Background()
	t.Run("minimal", func(t *testing.T) {
		fsys, err := local.NewFS(t.TempDir())
		be.NilErr(t, err)
		obj, err := ocfl.NewObject(ctx, fsys, ".", ocfl.ObjectWithID("new-object"))
		be.NilErr(t, err)
		be.False(t, obj.Exists())
		be.Zero(t, obj.InventoryDigest())
		_, err = obj.Update(
			ctx,
			&ocfl.Stage{State: ocfl.DigestMap{}, DigestAlgorithm: digest.SHA256},
			"new object",
			ocfl.User{Name: "Anna Karenina"},
			ocfl.UpdateWithOCFLSpec(ocfl.Spec1_0),
		)
		be.NilErr(t, err)
		be.True(t, obj.Exists())
		be.Nonzero(t, obj.InventoryDigest())
		be.Equal(t, "new object", obj.Version(0).Message())
		be.NilErr(t, ocfl.ValidateObject(ctx, obj.FS(), obj.Path()).Err())
	})
	t.Run("with wrong alg", func(t *testing.T) {
		fsys, err := local.NewFS(t.TempDir())
		be.NilErr(t, err)
		obj, err := ocfl.NewObject(ctx, fsys, ".", ocfl.ObjectWithID("new-object"))
		be.NilErr(t, err)
		be.False(t, obj.Exists())
		_, err = obj.Update(
			ctx,
			&ocfl.Stage{State: ocfl.DigestMap{}, DigestAlgorithm: digest.SHA512},
			"new object",
			ocfl.User{Name: "Anna Karenina"},
			ocfl.UpdateWithUnchangedVersionState(),
		)
		be.NilErr(t, err)
		_, err = obj.Update(
			ctx,
			&ocfl.Stage{State: ocfl.DigestMap{}, DigestAlgorithm: digest.SHA256},
			"new object",
			ocfl.User{Name: "Anna Karenina"},
			ocfl.UpdateWithUnchangedVersionState(),
		)
		be.Nonzero(t, err)
		be.In(t, "cannot change inventory's digest algorithm from previous value", err.Error())
	})
	t.Run("invalid spec", func(t *testing.T) {
		fsys, err := local.NewFS(t.TempDir())
		be.NilErr(t, err)
		obj, err := ocfl.NewObject(ctx, fsys, ".", ocfl.ObjectWithID("new-object"))
		be.NilErr(t, err)
		be.False(t, obj.Exists())
		_, err = obj.Update(
			ctx,
			&ocfl.Stage{State: ocfl.DigestMap{}, DigestAlgorithm: digest.SHA512},
			"new object",
			ocfl.User{Name: "Anna Karenina"},
			ocfl.UpdateWithOCFLSpec(ocfl.Spec1_1),
		)
		be.NilErr(t, err)
		_, err = obj.Update(
			ctx,
			&ocfl.Stage{State: ocfl.DigestMap{}, DigestAlgorithm: digest.SHA512},
			"new object",
			ocfl.User{Name: "Anna Karenina"},
			ocfl.UpdateWithOCFLSpec(ocfl.Spec1_0),
			ocfl.UpdateWithUnchangedVersionState(),
		)
		be.Nonzero(t, err)
	})
	t.Run("with extended digest algs", func(t *testing.T) {
		fsys, err := local.NewFS(t.TempDir())
		be.NilErr(t, err)
		obj, err := ocfl.NewObject(ctx, fsys, ".", ocfl.ObjectWithID("new-object"))
		be.NilErr(t, err)
		// update new object version from bytes:
		content := map[string][]byte{
			"README.txt": []byte("this is a test file"),
		}
		stage, err := ocfl.StageBytes(content, digest.SHA512, digest.SIZE)
		be.NilErr(t, err)
		_, err = obj.Update(
			ctx,
			stage, "new object",
			ocfl.User{Name: "Anna Karenina"},
		)
		be.NilErr(t, err)
		be.DeepEqual(t, []string{"size"}, obj.FixityAlgorithms())
		algReg := digest.NewAlgorithmRegistry(digest.SHA512, digest.SIZE)
		v := ocfl.ValidateObject(ctx, fsys, ".", ocfl.ValidationAlgorithms(algReg))
		be.NilErr(t, v.Err())
	})
}

func TestObject_UpdateFixtures(t *testing.T) {
	ctx := context.Background()
	for _, spec := range []string{`1.0`, `1.1`} {
		fixturesDir := filepath.Join(`testdata`, `object-fixtures`, spec, `good-objects`)
		fixtures, err := os.ReadDir(fixturesDir)
		be.NilErr(t, err)
		for _, dir := range fixtures {
			fixture := filepath.Join(fixturesDir, dir.Name())
			t.Run(fixture, func(t *testing.T) {
				tmpFS, err := local.NewFS(TempDirFixtureCopy(t, fixture))
				be.NilErr(t, err)
				obj, err := ocfl.NewObject(ctx, tmpFS, ".")
				be.NilErr(t, err)
				be.True(t, obj.Exists())

				newStage := obj.VersionStage(0)
				be.Nonzero(t, newStage)

				newContent, err := ocfl.StageBytes(map[string][]byte{
					"a-new-file": []byte("new stuff"),
				}, obj.DigestAlgorithm())
				be.NilErr(t, err)

				be.NilErr(t, newStage.Overlay(newContent))

				// do update
				_, err = obj.Update(ctx, newStage, "update", ocfl.User{Name: "Tristram Shandy"})
				be.NilErr(t, err)
				be.NilErr(t, ocfl.ValidateObject(ctx, obj.FS(), obj.Path()).Err())
				// check content
				newVersion, err := obj.VersionFS(ctx, 0)
				be.NilErr(t, err)
				cont, err := fs.ReadFile(newVersion, "a-new-file")
				be.NilErr(t, err)
				be.Equal(t, "new stuff", string(cont))
			})
		}
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
						if ok, desc := fixtureExpectedErrs(dir.Name(), result.Errors()...); !ok {
							t.Log(path.Join(spec, dir.Name())+":", desc)
						}
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

// for a fixture name and set of errors, returns if the errors include expected
// errors and string describing the difference between got and expected
func fixtureExpectedErrs(name string, errs ...error) (bool, string) {
	codeRegexp := regexp.MustCompile(`^E\d{3}$`)
	expCodes := map[string]bool{}
	gotCodes := map[string]bool{}
	for _, part := range strings.Split(name, "_") {
		if codeRegexp.MatchString(part) {
			expCodes[part] = true
		}
	}
	var gotExpected bool
	for _, e := range errs {
		var c = "??"
		var vErr *ocfl.ValidationError
		if errors.As(e, &vErr) {
			c = vErr.ValidationCode.Code
			gotCodes[c] = true
			if expCodes[c] {
				gotExpected = true
			}
		}
	}
	expKeys := make([]string, 0, len(expCodes))
	for k := range expCodes {
		expKeys = append(expKeys, k)
	}
	sort.Strings(expKeys)
	gotKeys := make([]string, 0, len(gotCodes))
	for k := range gotCodes {
		gotKeys = append(gotKeys, k)
	}
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
	return gotExpected, desc
}

func TempDirFixtureCopy(t *testing.T, fixture string) string {
	t.Helper()
	tmpDir := t.TempDir()
	if err := os.CopyFS(tmpDir, os.DirFS(fixture)); err != nil {
		t.Error(err)
	}
	return tmpDir
}
