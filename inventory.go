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
func (i *Inventory) Fprint(writer io.Writer) error {
	var j []byte
	var err error
	j, err = json.MarshalIndent(i, ``, "\t")
	if err != nil {
		return err
	}
	_, err = writer.Write(j)
	return err
}

// versionNames returns slice of version names
func (i *Inventory) versionNames() []string {
	var names []string
	for k := range i.Versions {
		names = append(names, k)
	}
	return names
}

func (i *Inventory) lastVersion() (Version, error) {
	var version Version
	vName := i.Head
	if vName == `` {
		return version, fmt.Errorf(`inventory has no Head`)
	}
	version, ok := i.Versions[vName]
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
