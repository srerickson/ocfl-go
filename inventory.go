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
	"os"
	"time"
)

const (
	inventoryType = `https://ocfl.io/1.0/spec/#inventory`
)

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

// // Manifest represents manifest elemenf of inventory.json. The manifest key
// // is a string representation of a checksum
// type Manifest map[string][]EPath

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

// Consistency checks that the inventory values are present and
// consistent. In the validation process, it does everything
// except validate the checksums in the manifest/fixity.
func (inv *Inventory) Consistency() error {

	// Validate Inventory Structure:
	if inv.ID == `` {
		return errors.New(`missing inventory ID: %s`)
	}
	if inv.Type != inventoryType {
		return fmt.Errorf(`invalid inventory type: %s`, inv.Type)
	}
	if inv.DigestAlgorithm == `` {
		return errors.New(`missing digestAlgorithm`)
	}
	if !stringIn(inv.DigestAlgorithm, digestAlgorithms[:]) {
		return fmt.Errorf(`invalid digestAlgorithm: %s`, inv.DigestAlgorithm)
	}
	if inv.Manifest == nil {
		return errors.New(`missing manifest`)
	}
	if inv.Versions == nil {
		return errors.New(`missing versions`)
	}

	// Validate Version Names in Inventory
	var versions = inv.versionNames()
	var padding int
	if len(inv.Versions) > 0 {
		padding = versionPadding(versions[0])
		for i := range versions {
			n, _ := versionGen(i+1, padding)
			if _, ok := inv.Versions[n]; !ok {
				return errors.New(`inconsistent or missing version names`)
			}
		}
	}

	// make sure every digest in version state is present in the manifest
	for vname := range inv.Versions {
		for digest := range inv.Versions[vname].State {
			if len(inv.Manifest.DigestPaths(digest)) == 0 {
				return fmt.Errorf(`digest missing from manifest: %s`, digest)
			}
		}
	}
	return nil
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
