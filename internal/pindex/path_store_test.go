package pindex_test

import (
	"testing"

	"github.com/srerickson/ocfl/internal/pindex"
)

func TestPathTree(t *testing.T) {
	ps := pindex.PathTree{}
	err := ps.Add("a/b/c/d/e.txt", "123")
	if err != nil {
		t.Error(err)
	}
	err = ps.Add("a/b/c/d/e/f.txt", "456")
	if err != nil {
		t.Error(err)
	}
	err = ps.Add("a/b/c/d/e.txt/f.txt", false)
	if err == nil {
		t.Error("expected path conflict")
	}
	err = ps.Add("A.txt", "321")
	if err != nil {
		t.Error(err)
	}
	err = ps.Add("a/b", "broken")
	if err == nil {
		t.Error("expected")
	}
	err = ps.Add("A.txt/thing", "broken")
	if err == nil {
		t.Error("expected")
	}
	val, err := ps.Get("a/b/c/d/e/f.txt")
	if err != nil {
		t.Error(err)
	}
	if val.(string) != "456" {
		t.Error("expected 456")
	}
	val, err = ps.Get("A.txt")
	if err != nil {
		t.Error(err)
	}
	if val.(string) != "321" {
		t.Error("expected 321")
	}
	_, err = ps.Get("")
	if err == nil {
		t.Error("expected invalid path")
	}
}
