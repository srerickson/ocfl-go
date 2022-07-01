package object_test

import (
	"context"
	"strings"
	"testing"

	"github.com/srerickson/ocfl/object"
)

func TestReadSidecar(t *testing.T) {
	valid := map[string]string{
		`abcd`: "abcd inventory.json\n",
		`9667917d5d55d0026a16dbadbba6737750bf08d5ce13d4c9e0a3aee6dd664bd935b0907c450ae09531f9d9f8d074e5702b10cbeed7acc3fcfe8e4e70dab590d8`: `9667917d5d55d0026a16dbadbba6737750bf08d5ce13d4c9e0a3aee6dd664bd935b0907c450ae09531f9d9f8d074e5702b10cbeed7acc3fcfe8e4e70dab590d8  inventory.json`,
	}
	for exp, test := range valid {
		got, err := object.ReadInventorySidecar(
			context.Background(),
			strings.NewReader(test),
		)
		if err != nil {
			t.Error(err)
		}
		if got != exp {
			t.Errorf("expected %s, got %s", exp, got)
		}
	}

}
