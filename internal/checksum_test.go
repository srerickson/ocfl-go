package internal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/srerickson/ocfl/internal"
)

func TestNewContentMap(t *testing.T) {
	dir := filepath.Join(goodObjPath, `spec-ex-full`)
	_, err := internal.ContentMap(os.DirFS(dir), `.`, internal.MD5)
	if err != nil {
		t.Fatal(err)
	}
}
