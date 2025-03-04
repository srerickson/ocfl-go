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

	"golang.org/x/exp/maps"
)

func TestObject(t *testing.T) {
	t.Run("Example", testObjectExample)
	t.Run("New", testNewObject)
	t.Run("Commit", testObjectCommit)
	t.Run("ValidateObject", testValidateObject)
	t.Run("ValidateFixtures", testValidateFixtures)
}

func testObjectExample(t *testing.T) {
	ctx := context.Background()
	tmpFS, err := local.NewFS(t.TempDir())
	be.NilErr(t, err)

	// open new-object-01, which doesn't exist
	obj, err := ocfl.NewObject(ctx, tmpFS, "new-object-01")
	be.NilErr(t, err)
	be.False(t, obj.Exists()) // the object doesn't exist yet
	be.Zero(t, obj.ID())      // its ID isn't set

	// commit new object version from bytes:
	v1Content := map[string][]byte{
		"README.txt": []byte("this is a test file"),
	}
	stage, err := ocfl.StageBytes(v1Content, digest.SHA512, digest.MD5)
	be.NilErr(t, err)
	err = obj.Commit(ctx, &ocfl.Commit{
		Spec:    ocfl.Spec1_0,
		ID:      "new-object-01",
		Stage:   stage,
		User:    ocfl.User{Name: "Mx. Robot"},
		Message: "first version",
	})
	be.NilErr(t, err)        // commit worked
	be.True(t, obj.Exists()) // the object was created

	// object has expected inventory values
	be.Equal(t, "new-object-01", obj.Inventory().ID())
	be.Nonzero(t, obj.Inventory().Version(1).State().PathMap()["README.txt"])

	// commit a new version and upgrade to OCFL v1.1
	v2Content := map[string][]byte{
		"README.txt":    []byte("this is a test file (v2)"),
		"new-data.csv":  []byte("1,2,3"),
		"docs/note.txt": []byte("this is a note"),
	}
	stage, err = ocfl.StageBytes(v2Content, digest.SHA512, digest.MD5)
	be.NilErr(t, err)
	err = obj.Commit(ctx, &ocfl.Commit{
		ID:      "new-object-01",
		Stage:   stage,
		User:    ocfl.User{Name: "Dr. Robot"},
		Message: "second version",
		Spec:    ocfl.Spec1_1,
	})
	be.NilErr(t, err)
	be.Equal(t, "new-object-01", obj.ID())
	be.Equal(t, ocfl.Spec1_1, obj.Inventory().Spec())
	be.Nonzero(t, obj.Inventory().Version(2).State().PathMap()["new-data.csv"])
	be.DeepEqual(t, []string{"md5"}, obj.Inventory().FixityAlgorithms())

	// open an object version to access files
	vfs, err := obj.OpenVersion(ctx, 0)
	be.NilErr(t, err)
	defer be.NilErr(t, vfs.Close())

	// vfs implements fs.FS for the version state
	be.NilErr(t, fstest.TestFS(vfs, maps.Keys(v2Content)...))

	// we can list files in a directory
	entries, err := fs.ReadDir(vfs, "docs")
	be.NilErr(t, err)
	be.Equal(t, 1, len(entries))

	// we can read files
	gotBytes, err := fs.ReadFile(vfs, "new-data.csv")
	be.NilErr(t, err)
	be.Equal(t, "1,2,3", string(gotBytes))

	// check that the object is valid
	be.NilErr(t, ocfl.ValidateObject(ctx, obj.FS(), obj.Path()).Err())
	// be.NilErr(t, result.Warning)

	// create a new object by forking new-object-01
	forkID := "new-object-02"
	fork := &ocfl.Commit{
		ID:      forkID,
		Stage:   vfs.Stage(),
		Message: vfs.Message(),
		User:    *vfs.User(),
	}
	forkObj, err := ocfl.NewObject(ctx, tmpFS, forkID)
	be.NilErr(t, err)
	be.NilErr(t, forkObj.Commit(ctx, fork))
	be.NilErr(t, ocfl.ValidateObject(ctx, forkObj.FS(), forkObj.Path()).Err())
	// TODO
	// roll-back an object to a previous version
	// interact with an object's extensions: list them, add an extension, remove an extension.
}

// OpenObject unit tests
func testNewObject(t *testing.T) {
	ctx := context.Background()
	fsys := ocflfs.DirFS(objectFixturesPath)
	type testCase struct {
		ctx    context.Context
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
			ctx:  ctx,
			fs:   fsys,
			path: "new-dir",
			expect: func(t *testing.T, obj *ocfl.Object, err error) {
				be.NilErr(t, err)
			},
		},
		"not existing, must exist": {
			ctx:  ctx,
			fs:   fsys,
			path: "new-dir",
			opts: []ocfl.ObjectOption{ocfl.ObjectMustExist()},
			expect: func(t *testing.T, obj *ocfl.Object, err error) {
				be.True(t, errors.Is(err, fs.ErrNotExist))
			},
		},
		"empty": {
			ctx:  ctx,
			fs:   fsys,
			path: "1.1/bad-objects/E003_E063_empty",
			opts: []ocfl.ObjectOption{},
			expect: func(t *testing.T, _ *ocfl.Object, err error) {
				be.Nonzero(t, err)
			},
		},
	}
	i := 0
	for name, tCase := range testCases {
		t.Run(fmt.Sprintf("%d-%s", i, name), func(t *testing.T) {
			if tCase.ctx == nil {
				tCase.ctx = ctx
			}
			obj, err := ocfl.NewObject(tCase.ctx, tCase.fs, tCase.path, tCase.opts...)
			tCase.expect(t, obj, err)
		})
		i++
	}
}

func testObjectCommit(t *testing.T) {
	ctx := context.Background()
	t.Run("minimal", func(t *testing.T) {
		fsys, err := local.NewFS(t.TempDir())
		be.NilErr(t, err)
		obj, err := ocfl.NewObject(ctx, fsys, ".")
		be.NilErr(t, err)
		be.False(t, obj.Exists())
		commit := &ocfl.Commit{
			ID:      "new-object",
			Stage:   &ocfl.Stage{State: ocfl.DigestMap{}, DigestAlgorithm: digest.SHA256},
			Message: "new object",
			User: ocfl.User{
				Name: "Anna Karenina",
			},
			Spec: ocfl.Spec1_0,
		}
		be.NilErr(t, obj.Commit(ctx, commit))
		be.True(t, obj.Exists())
		be.NilErr(t, ocfl.ValidateObject(ctx, obj.FS(), obj.Path()).Err())
	})
	t.Run("with wrong alg", func(t *testing.T) {
		fsys, err := local.NewFS(t.TempDir())
		be.NilErr(t, err)
		obj, err := ocfl.NewObject(ctx, fsys, ".")
		be.NilErr(t, err)
		be.False(t, obj.Exists())
		commit := &ocfl.Commit{
			ID:      "new-object",
			Stage:   &ocfl.Stage{State: ocfl.DigestMap{}, DigestAlgorithm: digest.SHA512},
			Message: "new object",
			User: ocfl.User{
				Name: "Anna Karenina",
			},
			Spec: ocfl.Spec1_0,
		}
		be.NilErr(t, obj.Commit(ctx, commit))
		commit.Stage.DigestAlgorithm = digest.SHA256
		err = obj.Commit(ctx, commit)
		be.True(t, err != nil)
		be.True(t, strings.Contains(err.Error(), "must use same digest algorithm as existing inventory"))
	})
	t.Run("with extended algorithm algs", func(t *testing.T) {
		fsys, err := local.NewFS(t.TempDir())
		be.NilErr(t, err)
		obj, err := ocfl.NewObject(ctx, fsys, ".")
		be.NilErr(t, err)
		// commit new object version from bytes:
		content := map[string][]byte{
			"README.txt": []byte("this is a test file"),
		}
		stage, err := ocfl.StageBytes(content, digest.SHA512, digest.SIZE)
		be.NilErr(t, err)
		commit := &ocfl.Commit{
			ID:      "new-object",
			Stage:   stage,
			Message: "new object",
			User: ocfl.User{
				Name: "Anna Karenina",
			},
			Spec: ocfl.Spec1_1,
		}
		be.NilErr(t, obj.Commit(ctx, commit))
		be.DeepEqual(t, []string{"size"}, obj.Inventory().FixityAlgorithms())
		algReg := digest.NewAlgorithmRegistry(digest.SHA512, digest.SIZE)
		v := ocfl.ValidateObject(ctx, fsys, ".", ocfl.ValidationAlgorithms(algReg))
		be.NilErr(t, v.Err())
	})
	t.Run("update fixtures", testUpdateFixtures)
}

func testUpdateFixtures(t *testing.T) {
	ctx := context.Background()
	for _, spec := range []string{`1.0`, `1.1`} {
		fixturesDir := filepath.Join(`testdata`, `object-fixtures`, spec, `good-objects`)
		fixtures, err := os.ReadDir(fixturesDir)
		be.NilErr(t, err)
		for _, dir := range fixtures {
			fixture := filepath.Join(fixturesDir, dir.Name())
			t.Run(fixture, func(t *testing.T) {
				objPath := "test-object"
				tmpFS, err := local.NewFS(t.TempDir())
				be.NilErr(t, err)
				be.NilErr(t, copyFixture(fixture, tmpFS, objPath))

				obj, err := ocfl.NewObject(ctx, tmpFS, objPath)
				be.NilErr(t, err)

				// new stage from the existing version and add a new file
				currentVersion, err := obj.OpenVersion(ctx, 0)
				be.NilErr(t, err)
				defer be.NilErr(t, currentVersion.Close())
				newContent, err := ocfl.StageBytes(map[string][]byte{
					"a-new-file": []byte("new stuff"),
				}, currentVersion.DigestAlgorithm())
				be.NilErr(t, err)
				stage := currentVersion.Stage()
				be.NilErr(t, stage.Overlay(newContent))

				// do commit
				be.NilErr(t, obj.Commit(ctx, &ocfl.Commit{
					Stage:   stage,
					Message: "update",
					User:    ocfl.User{Name: "Tristram Shandy"},
				}))
				be.NilErr(t, ocfl.ValidateObject(ctx, obj.FS(), obj.Path()).Err())
				// check content
				newVersion, err := obj.OpenVersion(ctx, 0)
				be.NilErr(t, err)
				defer be.NilErr(t, newVersion.Close())
				cont, err := fs.ReadFile(newVersion, "a-new-file")
				be.NilErr(t, err)
				be.Equal(t, "new stuff", string(cont))
			})
		}
	}
}

func testValidateObject(t *testing.T) {
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

func testValidateFixtures(t *testing.T) {
	ctx := context.Background()
	for _, spec := range []string{`1.0`, `1.1`} {
		t.Run(spec, func(t *testing.T) {
			fixturePath := filepath.Join(`testdata`, `object-fixtures`, spec)
			goodObjPath := filepath.Join(fixturePath, `good-objects`)
			badObjPath := filepath.Join(fixturePath, `bad-objects`)
			warnObjPath := filepath.Join(fixturePath, `warn-objects`)
			t.Run("Valid objects", func(t *testing.T) {
				fsys := ocflfs.NewFS(os.DirFS(goodObjPath))
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
				fsys := ocflfs.NewFS(os.DirFS(badObjPath))
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
				fsys := ocflfs.NewFS(os.DirFS(warnObjPath))
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

// creates a temporary directory and copies files from directory dir
// in fsys to the temporary directory. This is used to create writable
// object copies from fixtures
func copyFixture(fixture string, tmpFS ocflfs.WriteFS, tmpDir string) error {
	ctx := context.Background()
	fixFS := os.DirFS(fixture)
	return fs.WalkDir(fixFS, ".", func(name string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := fixFS.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = tmpFS.Write(ctx, path.Join(tmpDir, name), f)
		return err
	})
}
