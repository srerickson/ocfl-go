package extensions_test

import (
	"testing"

	"github.com/srerickson/ocfl-go/extensions"
)

func testLayoutExt(t *testing.T, l extensions.Layout, in, out string) {
	t.Helper()
	fn, err := l.NewFunc()
	if err != nil {
		t.Fatal(err)
	}
	got, err := fn(in)
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Fatalf("expected %s, got %s", out, got)
	}
}
