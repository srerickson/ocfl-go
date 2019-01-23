// Copyright 2019 Seth R. Erickson
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ocfl

import (
	"encoding/json"
	"testing"
)

func TestContentMap(t *testing.T) {
	var cm ContentMap
	if err := cm.Add(`fa`, `a-file`); err != nil {
		t.Error(err)
	}
	if l := cm.Len(); l != 1 {
		t.Errorf(`expected 1, got: %d`, l)
	}
	if err := cm.Add(`fa`, `another-file`); err != nil {
		t.Error(err)
	}
	if l := cm.LenDigest(`fa`); l != 2 {
		t.Errorf(`expected 2, got: %d`, l)
	}
	if err := cm.Rename(`a-file`, `another-file`); err == nil {
		t.Error(`expected an error`)
	}
	if err := cm.Rename(`a-file`, `b-file`); err != nil {
		t.Error(err)
	}
	if err := cm.Add(`e2`, `b-file`); err == nil {
		t.Error(`expected an error`)
	}
	if err := cm.Add(`e2`, `c-file`); err != nil {
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
	if l := cm.LenDigest(`fa`); l != 0 {
		t.Errorf(`expected 0, got: %d`, l)
	}
}

func TestContentMapAddDeduplicate(t *testing.T) {
	var cm ContentMap
	added, err := cm.AddDeduplicate(`aa`, `data.txt`)
	if err != nil {
		t.Error(err)
	}
	if added == false {
		t.Error(`expected added to be true`)
	}
	added, _ = cm.AddDeduplicate(`aa`, `data2.txt`)
	if added == true {
		t.Error(`expected added to be false`)
	}
	_, err = cm.AddDeduplicate(`ab`, `data.txt`)
	if err == nil {
		t.Error(`expected an error`)
	}
	_, err = cm.AddDeduplicate(`aa`, `../data2.txt`)
	if err == nil {
		t.Error(err)
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

func TestCleanPath(t *testing.T) {
	cm := ContentMap{}
	if err := cm.Add(`AA`, `.//uglypath`); err != nil {
		t.Error(err)
	}
	if err := cm.Add(`AB`, `../uglypath`); err == nil {
		t.Error(`expected an error`)
	}
	if d := cm.GetDigest(`uglypath`); d != `AA` {
		t.Errorf(`expected AA, got: %s`, d)
	}
	if err := cm.Rename(`./uglypath`, `.//another//ugly/path`); err != nil {
		t.Error(err)
	}
	if d, _ := cm.Remove(`another/ugly/path`); d != `AA` {
		t.Errorf(`expected AA, got: %s`, d)
	}
}

func TestCopyContentMap(t *testing.T) {
	a := ContentMap{}
	a.Add(`ab`, `file.txt`)
	b := a.Copy() // copy
	a.Remove(`file.txt`)
	if a.Len() == b.Len() {
		t.Error(`expected different lengths`)
	}

}
