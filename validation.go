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
	"log"
	"os"
	"path/filepath"
)

// Validator handles state for OCFL Object validation
type Validator struct {
	root      string
	inventory *Inventory
}

// ValidateObject validates the object at path
func ValidateObject(path string) error {
	var v Validator
	return v.ValidateObject(path)
}

// ValidateObject validates OCFL object located at path
func (v *Validator) ValidateObject(path string) error {
	obj, retErr := GetObject(path)
	if retErr != nil {
		log.Print(`error reading object: `, retErr)
		return retErr
	}
	v.root = obj.Path
	v.inventory = &obj.inventory
	alg := v.inventory.DigestAlgorithm

	// Validate Each Version Directory
	var files []os.FileInfo
	files, retErr = ioutil.ReadDir(path)
	if retErr != nil {
		log.Print(`error reading object: `, retErr)
		return retErr
	}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		if style := versionFormat(f.Name()); style != `` {
			if err := v.validateVersionDir(f.Name()); err != nil {
				retErr = err
			}
		}
	}
	// Manifest Checksum
	if err := v.inventory.Manifest.Validate(v.root, alg); err != nil {
		retErr = err
	}
	// Fixity Checksum
	for alg, manifest := range v.inventory.Fixity {
		if err := manifest.Validate(v.root, alg); err != nil {
			retErr = err
		}
	}
	return retErr
}

func (v *Validator) validateVersionDir(version string) error {
	invPath := filepath.Join(v.root, version, inventoryFileName)
	_, retErr := ReadValidateInventory(invPath)
	if os.IsNotExist(retErr) {
		log.Printf(`WARNING: Version %s has not inventory`, version)
	} else if retErr != nil {
		return retErr
	}
	// Check version content present in manifest
	contPath := filepath.Join(v.root, version, `content`)
	walk := func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		ePath, pathErr := filepath.Rel(v.root, path)
		if pathErr != nil {
			return pathErr
		}
		if v.inventory.Manifest.GetDigest(ePath) == `` {
			retErr = fmt.Errorf(`not in manifest: %s`, ePath)
			log.Print(retErr)
		}
		return nil
	}
	if err := filepath.Walk(contPath, walk); err != nil && !os.IsNotExist(err) {
		return err
	}
	return retErr
}
