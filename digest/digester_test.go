package digest_test

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"testing"

	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go/digest"
	"golang.org/x/crypto/blake2b"
)

func TestDigester(t *testing.T) {
	testDigester := func(t *testing.T, algs []string) {
		data := []byte("content")
		t.Helper()
		dig, err := digest.DefaultRegister().NewMultiDigester(algs...)
		if err != nil {
			t.Fatal(err)
		}
		// readfrom
		if _, err := io.Copy(dig, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		result := dig.Sums()
		if l := len(result); l != len(algs) {
			t.Fatalf("Sums() returned wrong number of entries: got=%d, expect=%d", l, len(algs))
		}
		expect := digest.Set{}
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
		result[digest.SHA1.ID()] = "invalid"
		err = result.Validate(bytes.NewReader(data))
		if err == nil {
			t.Error("Validate() didn't return an error for an invalid DigestSet")
		}
		var digestErr *digest.DigestError
		if !errors.As(err, &digestErr) {
			t.Error("Validate() didn't return a DigestErr as expected")
		}
		if digestErr.Alg != digest.SHA1.ID() {
			t.Error("Validate() returned an error with the wrong Alg value")
		}
		if digestErr.Expected != result[digest.SHA1.ID()] {
			t.Error("Validate() returned an error with wrong Expected value")
		}
	}
	t.Run("no algs", func(t *testing.T) {
		d := digest.NewMultiDigester()
		_, err := d.Write([]byte("test"))
		be.NilErr(t, err)
		be.Zero(t, len(d.Sums()))
	})
	t.Run("1 alg", func(t *testing.T) {
		testDigester(t, []string{digest.MD5.ID()})
	})
	t.Run("2 algs", func(t *testing.T) {
		testDigester(t, []string{digest.MD5.ID(), digest.BLAKE2B.ID()})
	})
}

func testAlg(algID string, val []byte) (string, error) {
	var h hash.Hash
	switch algID {
	case digest.SHA512.ID():
		h = sha512.New()
	case digest.SHA256.ID():
		h = sha256.New()
	case digest.SHA1.ID():
		h = sha1.New()
	case digest.MD5.ID():
		h = md5.New()
	case digest.BLAKE2B.ID():
		h, _ = blake2b.New512(nil)
	}
	d, err := digest.NewDigester(algID)
	if err != nil {
		return "", err
	}
	d.Write(val)
	h.Write(val)
	exp := hex.EncodeToString(h.Sum(nil))
	got := d.String()
	if exp != got {
		return exp, fmt.Errorf("%s value: got=%q, expected=%q", algID, got, exp)
	}
	return exp, nil
}
