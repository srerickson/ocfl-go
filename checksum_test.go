package ocfl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewContentMap(t *testing.T) {
	dir := filepath.Join(`test`, `fixtures`, `1.0`, `good-objects`, `spec-ex-full`)
	_, err := NewContentMap(os.DirFS(dir), `.`, MD5)
	if err != nil {
		t.Fatal(err)
	}
}
