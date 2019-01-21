package ocfl

import (
	"testing"
)

func TestConcurrentDigest(t *testing.T) {
	_, err := ConcurrentDigest(`test`, `sha1`)
	if err != nil {
		t.Error(err)
	}
	// b, err := json.MarshalIndent(cm, ``, `    `)
	// fmt.Println(string(b))
}
