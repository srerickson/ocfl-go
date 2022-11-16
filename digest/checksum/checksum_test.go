package checksum_test

import (
	"context"
	"errors"
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

func TestChecksum(t *testing.T) {
	ctx := context.Background()
	algsMD5SHA1 := []digest.Alg{digest.MD5(), digest.SHA1()}
	t.Run("minimal", func(t *testing.T) {
		setup := func(add func(name string, algs ...digest.Alg) error) error {
			return nil
		}
		cb := func(name string, results digest.Set, err error) error {
			return err
		}
		if err := checksum.Run(ctx, setup, cb); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("setup err", func(t *testing.T) {
		setup := func(add func(name string, algs ...digest.Alg) error) error {
			return errors.New("catch me")
		}
		cb := func(name string, results digest.Set, err error) error {
			return err
		}
		if err := checksum.Run(ctx, setup, cb); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("callback err", func(t *testing.T) {
		setup := func(add func(name string, algs ...digest.Alg) error) error {
			add(filepath.Join("test", "fixture", "hello.csv"))
			return nil
		}
		cb := func(name string, results digest.Set, err error) error {
			return errors.New("catch me")
		}
		if err := checksum.Run(ctx, setup, cb); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("minimal, one existing file, md5", func(t *testing.T) {
		setup := func(add func(name string, algs ...digest.Alg) error) error {
			add(filepath.Join("test", "fixture", "hello.csv"), digest.MD5())
			return nil
		}
		cb := func(name string, results digest.Set, err error) error {
			if err != nil {
				return err
			}
			if results[digest.MD5id] == "" {
				return errors.New("missing result")
			}
			return nil
		}
		if err := checksum.Run(ctx, setup, cb); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("minimal, one existing file, md5,sha1", func(t *testing.T) {
		setup := func(add func(name string, algs ...digest.Alg) error) error {
			add(filepath.Join("test", "fixture", "hello.csv"), algsMD5SHA1...)
			return nil
		}
		cb := func(name string, results digest.Set, err error) error {
			if err != nil {
				return err
			}
			if results[digest.MD5id] == "" {
				return errors.New("missing result")
			}
			if results[digest.SHA1id] == "" {
				return errors.New("missing result")
			}
			if len(results) > 2 {
				return errors.New("should ony have two results")
			}
			return nil
		}
		if err := checksum.Run(ctx, setup, cb); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("minimal, one existing file, no algs", func(t *testing.T) {
		setup := func(add func(name string, algs ...digest.Alg) error) error {
			add(filepath.Join("test", "fixture", "hello.csv"))
			return nil
		}
		cb := func(name string, results digest.Set, err error) error {
			if err != nil {
				return err
			}
			if len(results) > 0 {
				return errors.New("results should be empty")
			}
			return nil
		}
		if err := checksum.Run(ctx, setup, cb); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("minimal, one existing file, option algs, job algs", func(t *testing.T) {
		setup := func(add func(name string, algs ...digest.Alg) error) error {
			add(filepath.Join("test", "fixture", "hello.csv"), digest.MD5())
			return nil
		}
		cb := func(name string, results digest.Set, err error) error {
			if err != nil {
				return err
			}
			if len(results) != 2 {
				return errors.New("results should have two entries")
			}
			if results[digest.SHA1id] == "" {
				return errors.New("should have have SHA1")
			}
			if results[digest.MD5id] == "" {
				return errors.New("should have have MD5")
			}
			return nil
		}
		if err := checksum.Run(ctx, setup, cb, checksum.WithAlgs(digest.SHA1())); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("minimal, non-existing file, no algs", func(t *testing.T) {
		setup := func(add func(name string, algs ...digest.Alg) error) error {
			add("missingfile.txt")
			return nil
		}
		cb := func(name string, results digest.Set, err error) error {
			return err
		}
		if err := checksum.Run(ctx, setup, cb); err == nil {
			t.Fatal("expected an error: no file")
		}
	})
	t.Run("minimal, non-existing file, md5", func(t *testing.T) {
		setup := func(add func(name string, algs ...digest.Alg) error) error {
			add("missingfile.txt", digest.MD5())
			return nil
		}
		cb := func(name string, results digest.Set, err error) error {
			return err
		}
		if err := checksum.Run(ctx, setup, cb); err == nil {
			t.Fatal("expected an error: no file")
		}
	})
	t.Run("walk test dir", func(t *testing.T) {
		fsys := os.DirFS(`.`)
		results := map[string]string{}
		setup := func(add func(name string, algs ...digest.Alg) error) error {
			// walk fs
			walkFunc := func(p string, entr fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entr.Type().IsRegular() {
					return add(p, digest.MD5())
				}
				return nil
			}
			return fs.WalkDir(fsys, `test`, walkFunc)
		}
		cb := func(name string, sums digest.Set, err error) error {
			sum, ok := sums[digest.MD5().ID()]
			if !ok {
				return errors.New("expected md5")
			}
			results[name] = sum
			return nil
		}
		err := checksum.Run(ctx, setup, cb, checksum.WithFS(fsys), checksum.WithNumGos(4))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(testMD5Sums, results) {
			t.Fatalf("md5sums don't match expected values")
		}
	})
}
