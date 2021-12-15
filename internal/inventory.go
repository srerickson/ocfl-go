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

package internal

import (
	"encoding/json"
	"io"
	"time"

	"github.com/srerickson/ocfl/validation"
)

//var invSidecarRexp = regexp.MustCompile(`inventory\.json\.(\w+)`)

// Inventory represents contents of an OCFL Object's inventory.json file
type Inventory struct {
	ID               string               `json:"id"`
	Type             string               `json:"type"`
	DigestAlgorithm  string               `json:"digestAlgorithm"`
	Head             string               `json:"head"`
	ContentDirectory string               `json:"contentDirectory,omitempty"`
	Manifest         DigestMap            `json:"manifest"`
	Versions         map[string]*Version  `json:"versions"`
	Fixity           map[string]DigestMap `json:"fixity,omitempty"`
	digest           []byte               // digest of inventory file
}

// Version represent a version entryin inventory.json
type Version struct {
	Created time.Time `json:"created"`
	State   DigestMap `json:"state"`
	Message string    `json:"message,omitempty"`
	User    User      `json:"user,omitempty"`
}

// User represent a Version's user entry
type User struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}

func inventoryDefaults() *Inventory {
	return &Inventory{
		ContentDirectory: contentDir,
	}
}

func ReadInventory(file io.Reader) (*Inventory, error) {
	inv := inventoryDefaults()
	decoder := json.NewDecoder(file)

	// The OCFL spec (v1.0) allows uknown fields in some places (in
	// Versions, for examples). Using DisallowUnknownFields() would
	// invalidate some valid objects. Best to leave this disabled.
	// decoder.DisallowUnknownFields()

	err := decoder.Decode(inv)
	if err != nil {
		switch err := err.(type) {
		case *time.ParseError:
			return nil, validation.AsVErr(err, &validation.ErrE049)
		case *json.UnmarshalTypeError:
			if err.Field == "head" {
				return nil, validation.AsVErr(err, &validation.ErrE040)
			}
			if err.Field == `versions.message` {
				return nil, validation.AsVErr(err, &validation.ErrE094)
			}
			// Todo other special cases?
		}
		return nil, validation.AsVErr(err, &validation.ErrE033)
	}
	return inv, nil
}

// func ReadInventoryChecksum(file io.Reader, alg string) (*Inventory, error) {
// 	newH, err := newHash(alg)
// 	if err != nil {
// 		return nil, err
// 	}
// 	checksum := newH()
// 	reader := io.TeeReader(file, checksum)
// 	inv, err := ReadInventory(reader)
// 	if err != nil {
// 		return nil, err
// 	}
// 	inv.checksum = checksum.Sum(nil)
// 	return inv, nil
// }

// returns list of version directories that should be present in the object root
func (inv *Inventory) VersionDirs() []string {
	dirs := make([]string, 0, len(inv.Versions))
	for v := range inv.Versions {
		dirs = append(dirs, v)
	}
	return dirs
}

func (inv *Inventory) SidecarFile() string {
	return inventoryFile + "." + inv.DigestAlgorithm
}
