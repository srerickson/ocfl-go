package ocfl_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"path/filepath"
	"testing"

	"github.com/srerickson/ocfl-go"
	"golang.org/x/crypto/blake2b"
)

func TestDigestAlg(t *testing.T) {
	for _, alg := range ocfl.RegisteredAlgs() {
		if _, err := testAlg(alg, []byte("test data")); err != nil {
			t.Error(err)
		}
	}
}

func TestDigester(t *testing.T) {
	testDigester := func(t *testing.T, algs []string) {
		data := []byte("content")
		t.Helper()
		dig := ocfl.NewMultiDigester(algs...)
		// readfrom
		if _, err := io.Copy(dig, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		result := dig.Sums()
		if l := len(result); l != len(algs) {
			t.Fatalf("Sums() returned wrong number of entries: got=%d, expect=%d", l, len(algs))
		}
		expect := ocfl.DigestSet{}
		for alg := range result {
			var err error
			expect[alg], err = testAlg(alg, data)
			if err != nil {
				t.Error(err)
			}
			if expect[alg] != result[alg] {
				t.Errorf("ReadFrom() has unexpected result for %s: got=%q, expect=%q", alg, dig.Sum(alg), expect[alg])
			}
		}
		if err := result.Validate(bytes.NewReader(data)); err != nil {
			t.Error("Validate() unexpected error:", err)
		}
		// add invalid entry
		result[ocfl.SHA1] = "invalid"
		err := result.Validate(bytes.NewReader(data))
		if err == nil {
			t.Error("Validate() didn't return an error for an invalid DigestSet")
		}
		var digestErr *ocfl.DigestErr
		if !errors.As(err, &digestErr) {
			t.Error("Validate() didn't return a DigestErr as expected")
		}
		if digestErr.Alg != ocfl.SHA1 {
			t.Error("Validate() returned an error with the wrong Alg value")
		}
		if digestErr.Expected != result[ocfl.SHA1] {
			t.Error("Validate() returned an error with wrong Expected value")
		}
	}
	t.Run("nil algs", func(t *testing.T) {
		testDigester(t, nil)
	})
	t.Run("1 alg", func(t *testing.T) {
		testDigester(t, []string{ocfl.MD5})
	})
	t.Run("2 algs", func(t *testing.T) {
		testDigester(t, []string{ocfl.MD5, ocfl.BLAKE2B})
	})
}

func testAlg(alg string, val []byte) (string, error) {
	var h hash.Hash
	switch alg {
	case ocfl.SHA512:
		h = sha512.New()
	case ocfl.SHA256:
		h = sha256.New()
	case ocfl.SHA1:
		h = sha1.New()
	case ocfl.MD5:
		h = md5.New()
	case ocfl.BLAKE2B:
		h, _ = blake2b.New512(nil)
	}
	d := ocfl.NewDigester(alg)
	d.Write(val)
	h.Write(val)
	exp := hex.EncodeToString(h.Sum(nil))
	got := d.String()
	if exp != got {
		return exp, fmt.Errorf("%s value: got=%q, expected=%q", alg, got, exp)
	}
	return exp, nil
}

func TestDigestFS(t *testing.T) {
	var testMD5Sums = map[string]string{
		"hello.csv": "9d02fa6e9dd9f38327f7b213daa28be6",
	}
	fsys := ocfl.DirFS(filepath.Join("testdata", "content-fixture"))
	ctx := context.Background()
	algsMD5SHA1 := []string{ocfl.MD5, ocfl.SHA1}
	t.Run("minimal", func(t *testing.T) {
		setup := func(add func(name string, algs ...string) bool) {
		}
		cb := func(name string, results ocfl.DigestSet, err error) error {
			return err
		}
		if err := ocfl.DigestFS(ctx, fsys, setup, cb); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("callback err", func(t *testing.T) {
		setup := func(add func(name string, algs ...string) bool) {
			add(filepath.Join("missingfile"))
		}
		cb := func(name string, results ocfl.DigestSet, err error) error {
			return err
		}
		if err := ocfl.DigestFS(ctx, fsys, setup, cb); err == nil {
			t.Fatal("DigestFS() didn't return expected error")
		}
	})
	t.Run("minimal, one existing file, md5", func(t *testing.T) {
		setup := func(add func(name string, algs ...string) bool) {
			add("hello.csv", ocfl.MD5)
		}
		cb := func(name string, results ocfl.DigestSet, err error) error {
			if err != nil {
				return err
			}
			if results[ocfl.MD5] == "" {
				return errors.New("missing result")
			}
			return nil
		}
		if err := ocfl.DigestFS(ctx, fsys, setup, cb); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("minimal, one existing file, md5,sha1", func(t *testing.T) {
		setup := func(add func(name string, algs ...string) bool) {
			add("hello.csv", algsMD5SHA1...)
		}
		cb := func(name string, results ocfl.DigestSet, err error) error {
			if err != nil {
				return err
			}
			if results[ocfl.MD5] != testMD5Sums["hello.csv"] {
				return errors.New("wrong md5")
			}
			if results[ocfl.SHA1] == "" {
				return errors.New("missing result")
			}
			if len(results) > 2 {
				return errors.New("should ony have two results")
			}
			return nil
		}
		if err := ocfl.DigestFS(ctx, fsys, setup, cb); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("minimal, one existing file, no algs", func(t *testing.T) {
		setup := func(add func(name string, algs ...string) bool) {
			add("hello.csv")
		}
		cb := func(name string, results ocfl.DigestSet, err error) error {
			if err != nil {
				return err
			}
			if len(results) > 0 {
				return errors.New("results should be empty")
			}
			return nil
		}
		if err := ocfl.DigestFS(ctx, fsys, setup, cb); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("minimal, non-existing file, no algs", func(t *testing.T) {
		setup := func(add func(name string, algs ...string) bool) {
			add("missingfile")
		}
		cb := func(name string, results ocfl.DigestSet, err error) error {
			return err
		}
		if err := ocfl.DigestFS(ctx, fsys, setup, cb); err == nil {
			t.Fatal("DigestFS() didn't return expected error")
		}
	})
	t.Run("minimal, non-existing file, md5", func(t *testing.T) {
		setup := func(add func(name string, algs ...string) bool) {
			add("missingfile.txt", ocfl.MD5)
		}
		cb := func(name string, results ocfl.DigestSet, err error) error {
			return err
		}
		if err := ocfl.DigestFS(ctx, fsys, setup, cb); err == nil {
			t.Fatal("DigestFS() didn't return expected error")
		}
	})
}
