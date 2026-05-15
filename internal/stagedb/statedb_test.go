package stagedb_test

import (
	"path/filepath"
	"testing"

	stagedb "github.com/srerickson/ocfl-go/internal/stagedb"
	_ "modernc.org/sqlite"
)

func TestNewThing(t *testing.T) {
	tmpDir := t.TempDir()
	name := filepath.Join(tmpDir, "test.db")

	f, err := stagedb.Open(name)
	if err != nil {
		t.Fatal("opening stage file", err)
	}
	if err := f.Close(); err != nil {
		t.Fatal("closing stage file", err)
	}
}
