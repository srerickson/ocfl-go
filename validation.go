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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/srerickson/ocfl/namaste"
)

type Validator struct {
	root          string
	critical      []error
	warning       []error
	versionFormat string
	inventory     *Inventory
	checksums     map[string]string // cache of file -> digest
}

func ValidateObject(path string) error {
	var v Validator
	return v.ValidateObject(path)
}

func (v *Validator) init(root string) {
	*v = Validator{
		root:      root,
		checksums: map[string]string{},
	}
}

func (v *Validator) addCritical(err error) error {
	if err != nil {
		v.critical = append(v.critical, err)
	}
	return err
}

func (v *Validator) addWarning(err error) error {
	if err != nil {
		v.warning = append(v.warning, err)
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
	// TODO: Get Version Number to Compare to Inventory
	err = namaste.MatchTypePatternError(path, namasteObjectTValue)
	if err != nil {
		return v.addCritical(err)
	}

	// Validate Inventory
	v.inventory, err = v.validateInventory(inventoryFileName)
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
	if len(v.critical) > 0 {
		return v.critical[0]
	}
	return nil
}

func (v *Validator) validateInventory(name string) (*Inventory, error) {
	inv, err := ReadInventory(filepath.Join(v.root, name))
	if err != nil {
		return nil, v.addCritical(err)
	}

	// Validate Inventory Structure:
	if inv.ID == `` {
		v.addCritical(fmt.Errorf(`missing inventory ID: %s`, name))
	}
	if inv.Type != inventoryType {
		v.addCritical(fmt.Errorf(`bad type: %s`, inv.Type))
	}
	if inv.DigestAlgorithm == `` {
		v.addCritical(fmt.Errorf(`missing digestAlgorithm: %s`, name))
	}
	if !stringIn(inv.DigestAlgorithm, digestAlgorithms[:]) {
		v.addCritical(fmt.Errorf(`bad digestAlgorithm: %s`, inv.DigestAlgorithm))
	}
	if inv.Manifest == nil {
		v.addCritical(fmt.Errorf(`missing manifest: %s`, name))
	}
	if inv.Versions == nil {
		v.addCritical(fmt.Errorf(`missing version: %s`, name))
	}

	// Validate Version Names in Inventory
	var versions = inv.versionNames()
	var padding int
	if len(versions) > 0 {
		padding = versionPadding(versions[0])
		for i := range versions {
			n, _ := versionGen(i+1, padding)
			if _, ok := inv.Versions[n]; !ok {
				v.addCritical(fmt.Errorf(`inconsistent or missing version names: %s`, name))
				break
			}
		}
	} else {
		v.addWarning(fmt.Errorf(`inventory has no versions: %s`, name))
	}

	// Fixity
	for alg, manifest := range inv.Fixity {
		if err := v.validateContentMap(manifest, alg); err != nil {
			return nil, v.addCritical(err)
		}
	}
	// Manifest
	return &inv, v.validateContentMap(inv.Manifest, inv.DigestAlgorithm)
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
	inventoryPath := filepath.Join(version, inventoryFileName)
	// FIXME: don't need to checksum same file multiple times.
	if _, err := v.validateInventory(inventoryPath); err != nil {
		return v.addCritical(err)
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
					return fmt.Errorf(`not in manifest: %s`, ePath)
				}
			}
			return walkErr
		}
		return v.addCritical(filepath.Walk(contentPath, walk))
	}
	return nil
}
