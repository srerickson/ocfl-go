package ocfl

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"
)

func TestValidateObject(t *testing.T) {
	fixturePath := filepath.FromSlash(`test/fixtures/1.0/%s`)
	tests := map[bool]string{
		true:  fmt.Sprintf(fixturePath, `objects`),
		false: fmt.Sprintf(fixturePath, `bad-objects`),
	}
	for expected, objectRoot := range tests {
		fixtures, err := ioutil.ReadDir(objectRoot)
		if err != nil {
			t.Error(t)
		}
		for _, f := range fixtures {
			oPath := filepath.Join(objectRoot, f.Name())
			err := ValidateObject(oPath)
			if (err == nil) != expected {
				if expected {
					t.Errorf(`Expected ValidateObject() to return no error for %s, got: %s`, oPath, err)
				} else {
					t.Errorf(`Expected ValidateObject() to return an error for %s`, oPath)
				}
			}
		}

	}
}
