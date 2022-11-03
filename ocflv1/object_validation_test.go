package ocflv1_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/ocflv1"
	"github.com/srerickson/ocfl/validation"
)

var codeRegexp = regexp.MustCompile(`^E\d{3}$`)

func TestObjectValidation(t *testing.T) {
	specs := []string{`1.0`, `1.1`}
	for _, spec := range specs {
		t.Run(spec, func(t *testing.T) {
			fixturePath := filepath.Join(`..`, `testdata`, `object-fixtures`, spec)
			goodObjPath := filepath.Join(fixturePath, `good-objects`)
			badObjPath := filepath.Join(fixturePath, `bad-objects`)
			warnObjPath := filepath.Join(fixturePath, `warn-objects`)
			t.Run("Valid objects", func(t *testing.T) {
				fsys := ocfl.NewFS(os.DirFS(goodObjPath))
				goodObjects, err := fsys.ReadDir(context.Background(), ".")
				if err != nil {
					t.Fatal(err)
				}
				for _, dir := range goodObjects {
					t.Run(dir.Name(), func(t *testing.T) {
						_, result := ocflv1.ValidateObject(context.Background(), fsys, dir.Name())
						if result.Err() != nil {
							t.Error(`should be valid but got errors`)
							for _, err := range result.Fatal() {
								t.Errorf("\t - err: %s", err.Error())
							}
						}
						if len(result.Warn()) > 0 {
							t.Error(`should be no warnings`)
							for _, err := range result.Warn() {
								t.Errorf("\t - warn: %s", err.Error())
							}
						}
					})
				}
			})
			t.Run("Invalid objects", func(t *testing.T) {
				fsys := ocfl.NewFS(os.DirFS(badObjPath))
				badObjects, err := fsys.ReadDir(context.Background(), ".")
				if err != nil {
					t.Fatal(err)
				}
				for _, dir := range badObjects {
					if !dir.IsDir() {
						continue
					}
					t.Run(dir.Name(), func(t *testing.T) {
						_, result := ocflv1.ValidateObject(context.Background(), fsys, dir.Name())
						if result.Err() == nil {
							t.Error(`validated but shouldn't`)
							return
						}
						if ok, desc := fixtureExpectedErrs(dir.Name(), result.Fatal()...); !ok {
							t.Log(path.Join(spec, dir.Name())+":", desc)
						}
					})
				}
			})

			t.Run("Warning objects", func(t *testing.T) {
				fsys := ocfl.NewFS(os.DirFS(warnObjPath))
				warnObjects, err := fsys.ReadDir(context.Background(), ".")
				if err != nil {
					t.Fatal(err)
				}
				for _, dir := range warnObjects {
					t.Run(dir.Name(), func(t *testing.T) {
						_, result := ocflv1.ValidateObject(context.Background(), fsys, dir.Name())
						if result.Err() != nil {
							t.Error(`should be valid, but got errors:`)
							for _, err := range result.Fatal() {
								t.Logf("\t - err: %s", err.Error())
							}
						}
						if len(result.Warn()) == 0 {
							t.Error(`should have warning but got none.`)
						}
					})
				}
			})
		})
	}

}

func TestObjectValidatioSkipDigest(t *testing.T) {
	objPath := filepath.Join("..", "testdata", "object-fixtures", "1.0", "bad-objects", "E092_content_file_digest_mismatch")
	fsys := ocfl.DirFS(objPath)
	_, result := ocflv1.ValidateObject(context.Background(), fsys, ".")
	if err := result.Err(); err == nil {
		t.Fatal("expect an error if checking digests")
	}
	// validating this object without digest check should return no errors
	_, result = ocflv1.ValidateObject(context.Background(), fsys, ".", ocflv1.SkipDigests())
	if err := result.Err(); err != nil {
		t.Fatal(err)
	}
}

// for a fixture name and set of errors, returns if the errors include expected
// errors and string describing the difference between got and expected
func fixtureExpectedErrs(name string, errs ...error) (bool, string) {
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
		var err validation.ErrorCode
		if errors.As(e, &err) && err.OCFLRef() != nil {
			c = err.OCFLRef().Code
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
