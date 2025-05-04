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
	be.Equal(t, "new-object-01", obj.ID())
	sourceVersion := obj.Version(1)
	be.Nonzero(t, sourceVersion)
	be.Nonzero(t, sourceVersion.State.PathMap()["README.txt"])

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
	be.Equal(t, ocfl.Spec1_1, obj.Spec())
	be.Nonzero(t, obj.Version(2).State.PathMap()["new-data.csv"])
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
	fork := &ocfl.Commit{
		ID: forkID,
		Stage: &ocfl.Stage{
			State:           sourceVersion.State,
			DigestAlgorithm: obj.DigestAlgorithm(),
			FixitySource:    obj,
			ContentSource:   obj,
		},
		Message: sourceVersion.Message,
		User:    *sourceVersion.User,
	}
	forkObj, err := ocfl.NewObject(ctx, tmpFS, forkID)
	be.NilErr(t, err)
	be.NilErr(t, forkObj.Commit(ctx, fork))
	be.NilErr(t, ocfl.ValidateObject(ctx, forkObj.FS(), forkObj.Path()).Err())
	be.True(t, sourceVersion.State.Eq(forkObj.Version(0).State))
}

// OpenObject unit tests
func TestNewObject(t *testing.T) {
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

func TestObject_Commit(t *testing.T) {
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
			Spec:           ocfl.Spec1_0,
			AllowUnchanged: true,
		}
		be.NilErr(t, obj.Commit(ctx, commit))
		commit.Stage.DigestAlgorithm = digest.SHA256
		err = obj.Commit(ctx, commit)
		be.True(t, err != nil)
		be.In(t, "cannot change inventory's digest algorithm from previous value", err.Error())
	})
	t.Run("with extended digest algs", func(t *testing.T) {
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
				objPath := "test-object"
				tmpFS, err := local.NewFS(t.TempDir())
				be.NilErr(t, err)
				be.NilErr(t, copyFixture(fixture, tmpFS, objPath))

				obj, err := ocfl.NewObject(ctx, tmpFS, objPath)
				be.NilErr(t, err)
				be.True(t, obj.Exists())

				newStage := obj.VersionStage(0)
				be.Nonzero(t, newStage)

				newContent, err := ocfl.StageBytes(map[string][]byte{
					"a-new-file": []byte("new stuff"),
				}, obj.DigestAlgorithm())
				be.NilErr(t, err)

				be.NilErr(t, newStage.Overlay(newContent))

				// do commit
				be.NilErr(t, obj.Commit(ctx, &ocfl.Commit{
					Stage:   newStage,
					Message: "update",
					User:    ocfl.User{Name: "Tristram Shandy"},
				}))
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
					err = fstest.TestFS(logicalFS, ver.State.AllPaths()...)
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
