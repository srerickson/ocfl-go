package namaste

import (
	"path/filepath"
	"testing"
)

func TestSearch(t *testing.T) {
	fixtures := filepath.FromSlash(`../test/fixtures`)
	results, err := SearchTypePattern(fixtures, `ocfl_object.*`)
	if err != nil {
		t.Error(err)
	}
	if len(results) != 6 {
		t.Error(`expected 6 results from SearchTypePattern`)
	}
}
