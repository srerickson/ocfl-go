package checksum_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/digest/checksum"
)

var testMD5Sums = map[string]string{
	"test/fixture/folder1/folder2/sculpture-stone-face-head-888027.jpg": "e8c078f0e4ad79b16fcb618a3790c2df",
	"test/fixture/folder1/folder2/file2.txt":                            "d41d8cd98f00b204e9800998ecf8427e",
	"test/fixture/folder1/file.txt":                                     "d41d8cd98f00b204e9800998ecf8427e",
	"test/fixture/hello.csv":                                            "9d02fa6e9dd9f38327f7b213daa28be6",
}

func TestPipe(t *testing.T) {
	algs := []digest.Alg{digest.MD5}
	t.Run("zero values, zero files", func(t *testing.T) {
		setup := func(add checksum.AddFunc) error {
			return nil
		}
		cb := func(name string, results digest.Set, err error) error {
			return err
		}
		if err := checksum.Run(setup, cb); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("zero values, one file", func(t *testing.T) {
		setup := func(add checksum.AddFunc) error {
			add(filepath.Join("test", "fixture", "hello.csv"), algs)
			return nil
		}
		cb := func(name string, results digest.Set, err error) error {
			return err
		}
		if err := checksum.Run(setup, cb); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("walk test dir", func(t *testing.T) {
		fsys := os.DirFS(`.`)
		results := map[string]string{}
		setup := func(add checksum.AddFunc) error {
			// walk fs
			walkFunc := func(p string, entr fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entr.Type().IsRegular() {
					if !add(p, algs) {
						return fmt.Errorf("%s not added", p)
					}
				}
				return nil
			}
			return fs.WalkDir(fsys, `test`, walkFunc)
		}
		cb := func(name string, sums digest.Set, err error) error {
			sum, ok := sums[digest.MD5]
			if !ok {
				return errors.New("expected md5")
			}
			results[name] = sum
			return nil
		}
		err := checksum.Run(setup, cb, checksum.WithFS(fsys), checksum.WithNumGos(4))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(testMD5Sums, results) {
			t.Fatalf("md5sums don't match expected values")
		}
	})
	t.Run("callback error", func(t *testing.T) {
		fsys := os.DirFS(`.`)
		cb := func(name string, sums digest.Set, err error) error {
			return errors.New("catch me")
		}
		setup := func(add checksum.AddFunc) error {
			if !add("test/fixture/hello.csv", algs) {
				return fmt.Errorf("add failed")
			}
			return nil
		}
		err := checksum.Run(setup, cb, checksum.WithFS(fsys))
		if err == nil {
			t.Error("expected error from close")
		} else if err.Error() != "catch me" {
			t.Error("expected: catch me")
		}
	})

	t.Run("setup error", func(t *testing.T) {
		cb := func(name string, sums digest.Set, err error) error {
			return err
		}
		setup := func(add checksum.AddFunc) error {
			return errors.New("catch me")
		}
		err := checksum.Run(setup, cb)
		if err == nil {
			t.Error("expected error from close")
		} else if err.Error() != "catch me" {
			t.Error("expected: catch me")
		}
	})

}
