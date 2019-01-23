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
	"regexp"
)

var invSidecarRexp = regexp.MustCompile(`inventory\.json\.(\w+)`)

// Validator handles state for OCFL Object validation
type Validator struct {
	root      string
	lastErr   error
	inventory *Inventory
}

// ValidateObject validates the object at path
func ValidateObject(path string) error {
	var v Validator
	return v.ValidateObject(path)
}

// ValidateObject validates OCFL object located at path
func (v *Validator) ValidateObject(path string) error {
	obj, err := GetObject(path)
	if err != nil {
		log.Print(`error reading object: `, err)
		return err
	}
	v.root = obj.Path
	v.inventory = &obj.inventory

	// Validate Each Version Directory
	files, err := ioutil.ReadDir(path)
	if err != nil {
		log.Print(`error reading object: `, err)
		return err
	}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		if style := versionFormat(f.Name()); style != `` {
			v.validateVersionDir(f.Name())
		}
	}
	// Manifest Checksum
	v.inventory.Manifest.Validate(v.root, v.inventory.DigestAlgorithm)
	// Fixity Checksum
	for alg, manifest := range v.inventory.Fixity {
		manifest.Validate(v.root, alg)
	}
	return v.lastErr
}

func (v *Validator) validateVersionDir(version string) error {
	invPath := filepath.Join(v.root, version, inventoryFileName)
	_, err := ReadValidateInventory(invPath)
	if os.IsNotExist(err) {
		log.Printf(`WARNING: Version %s has not inventory`, version)
	} else if err != nil {
		return err
	}
	// Check version content present in manifest
	var returnErr error
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
			returnErr = fmt.Errorf(`not in manifest: %s`, ePath)
			log.Print(returnErr)
		}
		return nil
	}
	err = filepath.Walk(contPath, walk)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return returnErr
}
