package ocfl

import (
	"encoding/json"
	"testing"
)

func TestContentMap(t *testing.T) {
	var cm ContentMap
	if err := cm.Add(`fd4305341e6939cae02eb767176427d9`, `a-file`); err != nil {
		t.Error(err)
	}
	if l := cm.Len(); l != 1 {
		t.Errorf(`expected 1, got: %d`, l)
	}
	if err := cm.Add(`fd4305341e6939cae02eb767176427d9`, `another-file`); err != nil {
		t.Error(err)
	}
	if l := cm.Len(); l != 2 {
		t.Errorf(`expected 2, got: %d`, l)
	}
	if err := cm.Rename(`a-file`, `another-file`); err == nil {
		t.Error(`expected an error`)
	}
	if err := cm.Rename(`a-file`, `b-file`); err != nil {
		t.Error(err)
	}
	if err := cm.Add(`ed4305341e6939cae02eb767176427d2`, `b-file`); err == nil {
		t.Error(`expected an error`)
	}
	if err := cm.Add(`ed4305341e6939cae02eb767176427d2`, `c-file`); err != nil {
		t.Error(err)
	}
	if _, err := cm.Remove(`b-file`); err != nil {
		t.Error(err)
	}
	if _, err := cm.Remove(`another-file`); err != nil {
		t.Error(err)
	}
	if _, err := cm.Remove(`c-file`); err != nil {
		t.Error(err)
	}
	if _, err := cm.Remove(`no-file`); err == nil {
		t.Error(`expected an error`)
	}
	if l := cm.Len(); l != 0 {
		t.Errorf(`expected 0, got: %d`, l)
	}
}

func TestContentMapJSON(t *testing.T) {
	var cm ContentMap
	jsonData := `{"fd4305341e6939cae02eb767176427d9":["file.txt","test.txt"]}`
	if err := json.Unmarshal([]byte(jsonData), &cm); err != nil {
		t.Error(err)
	}
	if cm.Len() != 2 {
		t.Error(`expected 2, got:`, cm.DigestPaths(`fd4305341e6939cae02eb767176427d9`))
	}
	if err := json.Unmarshal([]byte(`{"z":["test.txt"]}`), &cm); err == nil {
		t.Errorf(`expected an error`)
	}
	var cm2 ContentMap
	cm2.Add(`fd4305341e6939cae02eb767176427d9`, `test.txt`)
	cm2.Add(`fd4305341e6939cae02eb767176427d9`, `file.txt`)
	jsonResult, err := json.Marshal(cm2)
	if err != nil {
		t.Error(err)
	}
	if string(jsonResult) != jsonData {
		t.Errorf(`expected %s, but got: %s`, jsonData, jsonResult)
	}
	var cm3 ContentMap
	cm3.Add(`z`, `file.txt`)
	if jsonResult, err = json.Marshal(cm3); err == nil {
		t.Errorf(`expected an error but got: %s`, jsonResult)
	}

}

func TestDigestJSON(t *testing.T) {
	var d1 Digest
	if err := json.Unmarshal([]byte(`"1234"`), &d1); err != nil {
		t.Error(err)
	}
	if err := json.Unmarshal([]byte(`"x1234"`), &d1); err == nil {
		t.Errorf(`expected error, got: %s`, d1)
	}
	var d2 Digest = `bad digest`
	if j, err := json.Marshal(d2); err == nil {
		t.Errorf(`expected error, got: %s`, string(j))
	}
}

func TestPathJSON(t *testing.T) {
	var p1 Path
	if err := json.Unmarshal([]byte(`"test/tmp.txt"`), &p1); err != nil {
		t.Error(err)
	}
	if err := json.Unmarshal([]byte(`"../tmp.txt"`), &p1); err == nil {
		t.Errorf(`expected error, got: %s`, p1)
	}
	var p2 Path = `/abs/path.txt`
	if j, err := json.Marshal(p2); err == nil {
		t.Errorf(`expected error, got: %s`, string(j))
	}
}
