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

func TestContentMapValidate(t *testing.T) {
	cm, err := ConcurrentDigest(`test`, SHA1)
	if err != nil {
		t.Error(err)
	}
	err = cm.ValidateHandleErr(`test`, SHA1, nil)
	if err != nil {
		t.Error(err)
	}
	err = cm.ValidateHandleErr(`test2`, SHA1, nil)
	if err == nil {
		t.Error(`expected error`)
	}
	err = cm.ValidateHandleErr(`test`, MD5, nil)
	if err == nil {
		t.Error(`expected error`)
	}
}
