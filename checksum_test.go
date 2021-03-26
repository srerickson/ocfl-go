package ocfl

import (
	"log"
	"os"
	"path/filepath"
	"testing"
)

func TestNewContentMap(t *testing.T) {
	dir := filepath.Join(`test`, `fixtures`, `1.0`, `good-objects`, `spec-ex-full`)
	cm, err := NewContentMap(os.DirFS(dir), `.`, MD5)
	if err != nil {
		t.Fatal(err)
	}
	log.Println(cm)
}
