package digest_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/srerickson/ocfl/digest"
)

func testDigester(t *testing.T, b []byte, algs []digest.Alg) {
	t.Helper()
	dig := digest.NewDigester(algs...)
	size := len(b)

	// readfrom
	gotsize, err := dig.ReadFrom(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if gotsize != int64(size) {
		t.Fatalf("digester ReadFrom returned %d, expected %d", gotsize, size)
	}
	if l := len(dig.Sums()); l != len(algs) {
		t.Fatalf("expected %d sums in results, got %d", len(algs), l)
	}

	// reader
	gotsize, err = io.Copy(&bytes.Buffer{}, dig.Reader(bytes.NewReader(b)))
	if err != nil {
		t.Fatal(err)
	}
	if gotsize != int64(size) {
		t.Fatalf("reading from digester Reader returned %d, expected %d", gotsize, size)
	}
	if l := len(dig.Sums()); l != len(algs) {
		t.Fatalf("expected %d sums in results, got %d", len(algs), l)
	}
}

func TestDigester(t *testing.T) {
	t.Run("nil algs", func(t *testing.T) {
		b := []byte("content")
		testDigester(t, b, nil)
	})
	t.Run("1 alg", func(t *testing.T) {
		b := []byte("content")
		testDigester(t, b, []digest.Alg{digest.MD5()})
	})
	t.Run("2 algs", func(t *testing.T) {
		b := []byte("content")
		testDigester(t, b, []digest.Alg{digest.MD5(), digest.BLAKE2B()})
	})
}
