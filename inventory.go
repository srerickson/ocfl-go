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
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	inventoryType = `https://ocfl.io/1.0/spec/#inventory`
)

var invSidecarRexp = regexp.MustCompile(`inventory\.json\.(\w+)`)

// Inventory represents contents of an OCFL Object's inventory.json file
type Inventory struct {
	ID              string                `json:"id"`
	Type            string                `json:"type"`
	DigestAlgorithm string                `json:"digestAlgorithm"`
	Head            string                `json:"head"`
	Manifest        ContentMap            `json:"manifest"`
	Versions        map[string]Version    `json:"versions"`
	Fixity          map[string]ContentMap `json:"fixity"`
}

// Version represent a version entryin inventory.json
type Version struct {
	Created time.Time  `json:"created"`
	Message string     `json:"message"`
	User    User       `json:"user"`
	State   ContentMap `json:"state"`
}

// User represent a Version's user entry
type User struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// NewInventory returns a new, empty inventory with default values
func NewInventory(id string) Inventory {
	return Inventory{
		ID:              id,
		Type:            inventoryType,
		DigestAlgorithm: defaultAlgorithm,
		Versions:        map[string]Version{},
		Manifest:        ContentMap{},
		Fixity:          map[string]ContentMap{},
	}
}

// ReadInventory returns Inventory from json file at path
func ReadInventory(path string) (Inventory, error) {
	var inv Inventory
	var file *os.File
	var invJSON []byte
	var err error
	if file, err = os.Open(path); err != nil {
		return inv, err
	}
	defer file.Close()
	if invJSON, err = ioutil.ReadAll(file); err != nil {
		return inv, err
	}
	if err = json.Unmarshal(invJSON, &inv); err != nil {
		return inv, err
	}
	return inv, nil
}

// ReadValidateInventory is same as ReadInventory with the addition
// that it validates the inventory file's checksum against a sidecar file
// (typically inventory.json.sha512), and it returns errors from
// Inventory.Consistency(). Note, it *does not* validate the checksums
// of the files listed in the manifest/fixity sections of the inventory.
func ReadValidateInventory(path string) (Inventory, error) {
	inv, err := ReadInventory(path)
	if err != nil {
		return inv, err
	}
	// Validate Inventory File Checksum
	var sidecarPath string
	var sidecarAlg string
	fList, err := ioutil.ReadDir(filepath.Dir(path))
	if err != nil {
		return inv, err
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
		return inv, errors.New(`missing inventory checksum file`)
	}
	readBytes, err := ioutil.ReadFile(sidecarPath)
	if err != nil {
		return inv, err
	}
	expectedSum := strings.Trim(string(readBytes), "\r\n ")
	sum, err := Checksum(sidecarAlg, path)
	if err != nil || expectedSum != sum {
		return inv, errors.New(`failed to validate inventory file checksum`)
	}
	return inv, inv.Consistency()
}

// Consistency checks that the inventory values are present and
// consistent. As part of the validation process, it does everything
// except validate the checksums in the manifest/fixity.
func (inv *Inventory) Consistency() error {
	var lastErr error
	setErrf := func(s string, v ...interface{}) {
		err := fmt.Errorf(s, v...)
		lastErr = err
		log.Println(err)
	}
	// Validate Inventory Structure:
	if inv.ID == `` {
		setErrf(`missing inventory ID`)
	}
	if inv.Type != inventoryType {
		setErrf(`invalid inventory type: %s`, inv.Type)
	}
	if inv.DigestAlgorithm == `` {
		setErrf(`missing digestAlgorithm`)
	} else if !stringIn(inv.DigestAlgorithm, digestAlgorithms[:]) {
		setErrf(`invalid digestAlgorithm: %s`, inv.DigestAlgorithm)
	}
	if inv.Manifest == nil {
		setErrf(`missing manifest`)
	}
	if inv.Versions == nil {
		setErrf(`missing versions`)
	}
	// Validate Version Names in Inventory
	var versions = inv.versionNames()
	var padding int
	if len(inv.Versions) > 0 {
		padding = versionPadding(versions[0])
		for i := range versions {
			n, _ := versionGen(i+1, padding)
			if _, ok := inv.Versions[n]; !ok {
				setErrf(`inconsistent or missing version names`)
			}
		}
	}
	// make sure every digest in version state is present in the manifest
	for vname := range inv.Versions {
		for digest := range inv.Versions[vname].State {
			if inv.Manifest.LenDigest(digest) == 0 {
				setErrf(`digest missing from manifest: %s`, digest)
			}
		}
	}
	return lastErr
}

// Fprint prints the inventory to writer as json
func (inv *Inventory) Fprint(writer io.Writer) error {
	var j []byte
	var err error
	j, err = json.MarshalIndent(inv, ``, "\t")
	if err != nil {
		return err
	}
	_, err = writer.Write(j)
	return err
}

// versionNames returns slice of version names
func (inv *Inventory) versionNames() []string {
	var names []string
	for k := range inv.Versions {
		names = append(names, k)
	}
	return names
}

func (inv *Inventory) lastVersion() (Version, error) {
	var version Version
	vName := inv.Head
	if vName == `` {
		return version, nil
	}
	version, ok := inv.Versions[vName]
	if !ok {
		return version, fmt.Errorf(`version not found: %s`, vName)
	}
	return version, nil
}

// NewVersion returns a new, empty Version
func NewVersion() Version {
	return Version{
		State: ContentMap{},
	}
}

// NewUser returns a new User with given name and address
func NewUser(name string, address string) User {
	return User{
		Name:    name,
		Address: address,
	}
}
