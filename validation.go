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
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/srerickson/ocfl/namaste"
)

var invSidecarRexp = regexp.MustCompile(`inventory\.json\.(\w+)`)

// Validator handles state for OCFL Object validation
type Validator struct {
	HandleErr func(err error)
	HandleWrn func(err error)
	root      string
	lastErr   error
	inventory *Inventory
}

// ValidateObject validates the object at path
func ValidateObject(path string) error {
	var v Validator
	return v.ValidateObject(path)
}

func (v *Validator) init(root string) {
	*v = Validator{
		root:      root,
		HandleErr: v.HandleErr,
		HandleWrn: v.HandleWrn,
	}
}

func (v *Validator) addCritical(err error) error {
	if err != nil {
		v.lastErr = err
		if v.HandleErr != nil {
			v.HandleErr(err)
		}
	}
	return err
}

func (v *Validator) addWarning(err error) error {
	if err != nil {
		if v.HandleWrn != nil {
			v.HandleWrn(err)
		}
	}
	return err
}

// ValidateObject validates OCFL object located at path
func (v *Validator) ValidateObject(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return v.addCritical(err)
	}
	v.init(absPath)

	// Object Conformance Declaration
	err = namaste.MatchTypePatternError(path, namasteObjectTValue)
	if err != nil {
		return v.addCritical(err)
	}

	// Validate Inventory Structure (not checksum)
	v.inventory, err = v.readInventory(inventoryFileName)
	if err != nil {
		return v.addCritical(err)
	}

	// Version Directories
	if files, err := ioutil.ReadDir(path); err != nil {
		v.addCritical(err)
	} else {
		for _, f := range files {
			if !f.IsDir() {
				continue
			}
			if style := versionFormat(f.Name()); style != `` {
				v.validateObjectVersionDir(f.Name())
			}
		}

	}

	// Manifest Checksum
	v.validateContentMap(v.inventory.Manifest, v.inventory.DigestAlgorithm)

	// Fixity Checksum
	for alg, manifest := range v.inventory.Fixity {
		v.validateContentMap(manifest, alg)
	}

	return v.lastErr
}

func (v *Validator) readInventory(name string) (*Inventory, error) {
	path := filepath.Join(v.root, name)
	inv, err := ReadInventory(path)
	if err != nil {
		return nil, err
	}
	// Check Inventory Consistency
	err = inv.Consistency()
	if err != nil {
		return nil, err
	}

	// Validate Inventory File Checksum
	var sidecarPath string
	var sidecarAlg string
	fList, err := ioutil.ReadDir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	for _, info := range fList {
		matches := invSidecarRexp.FindStringSubmatch(info.Name())
		if len(matches) > 1 {
			sidecarAlg = matches[1]
			sidecarPath = fmt.Sprint(path, `.`, sidecarAlg)
			break
		}
	}
	if sidecarPath == `` {
		return nil, errors.New(`missing inventory checksum file`)
	}
	readBytes, err := ioutil.ReadFile(sidecarPath)
	if err != nil {
		return nil, err
	}
	expectedSum := strings.Trim(string(readBytes), "\n ")
	sum, err := Checksum(sidecarAlg, path)
	if err != nil || expectedSum != sum {
		return nil, errors.New(`failed to validate inventory file checksum`)
	}
	return &inv, nil
}

func (v *Validator) validateContentMap(cm ContentMap, alg string) error {
	for expectedSum := range cm {
		for path := range cm[expectedSum] {
			fullPath := filepath.Join(v.root, string(path))
			info, err := os.Stat(fullPath)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return v.addCritical(fmt.Errorf("irregular file in manifest: %s", path))
			}
			gotSum, err := Checksum(alg, fullPath)
			if err != nil {
				return v.addCritical(err)
			}
			if expectedSum != Digest(gotSum) {
				return v.addCritical(fmt.Errorf("checksum failed for %s", path))
			}
		}
	}
	return nil
}

func (v *Validator) validateObjectVersionDir(version string) error {
	_, err := v.readInventory(filepath.Join(version, inventoryFileName))
	if err != nil {
		return v.addWarning(err)
	}
	contentPath := filepath.Join(v.root, version, `content`)
	if i, statErr := os.Stat(contentPath); statErr == nil && i.IsDir() {
		// Walk Version content, check all files present in manifest
		walk := func(path string, info os.FileInfo, walkErr error) error {
			if walkErr == nil && info.Mode().IsRegular() {
				ePath, pathErr := filepath.Rel(v.root, path)
				if pathErr != nil {
					return pathErr
				}
				if v.inventory.Manifest.GetDigest(Path(ePath)) == `` {
					v.addCritical(fmt.Errorf(`not in manifest: %s`, ePath))
				}
			}
			return walkErr
		}
		return v.addCritical(filepath.Walk(contentPath, walk))
	}
	return nil
}
