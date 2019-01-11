package ocfl

import "testing"

func TestVersionHelpers(t *testing.T) {

	if v, _ := versionGen(31, 3); v != `v031` {
		t.Errorf(`expected v031, got: %s: `, v)
	}

	if _, err := versionGen(31, 2); err == nil {
		t.Error(`expected an error`)
	}

	if _, err := nextVersionLike(``); err == nil {
		t.Error(`expected an error`)
	}

	if _, err := nextVersionLike(`adf`); err == nil {
		t.Error(`expected an error`)
	}

	if v, err := nextVersionLike(`v099`); err == nil {
		t.Errorf(`expected a padding overflow error, got: %s`, v)
	}

	if v, _ := nextVersionLike(`v1`); v != `v2` {
		t.Errorf(`expected v2, got: %s`, v)
	}

	if v, _ := nextVersionLike(`v01`); v != `v02` {
		t.Errorf(`expected v02, got: %s`, v)
	}
}
