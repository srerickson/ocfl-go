package ocfl_test

import (
	"testing"

	"github.com/srerickson/ocfl"
)

func TestVStateChanges(t *testing.T) {
	a := &ocfl.VState{
		State: map[string][]string{
			"file-1": {`file-1`},
			"file-2": {`file-2`},
		},
	}
	b := &ocfl.VState{
		State: map[string][]string{
			"file-2": {`file-2`},
		},
	}
	ch := a.Diff(b)
	if l := len(ch.Del); l != 1 {
		t.Fatalf(`exected 1 entry for .Del, got %d`, l)
	}

}
