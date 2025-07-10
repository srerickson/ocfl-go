package local_test

import (
	"testing"

	"github.com/srerickson/ocfl-go/fs/local"
)

func TestFS_Eq(t *testing.T) {
	t.Run("local.FS equality", func(t *testing.T) {
		// Create two FS with the same path
		fs1, err := local.NewFS("/tmp")
		if err != nil {
			t.Fatal(err)
		}
		fs2, err := local.NewFS("/tmp")
		if err != nil {
			t.Fatal(err)
		}
		
		// They should be equal
		if !fs1.Eq(fs2) {
			t.Error("Expected fs1.Eq(fs2) to be true for same paths")
		}
		if !fs2.Eq(fs1) {
			t.Error("Expected fs2.Eq(fs1) to be true for same paths")
		}
		
		// Create FS with different path
		fs3, err := local.NewFS("/")
		if err != nil {
			t.Fatal(err)
		}
		
		// They should not be equal
		if fs1.Eq(fs3) {
			t.Error("Expected fs1.Eq(fs3) to be false for different paths")
		}
		if fs3.Eq(fs1) {
			t.Error("Expected fs3.Eq(fs1) to be false for different paths")
		}
	})
}