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
	"time"
)

const (
//inventoryType = `https://ocfl.io/1.0/spec/#inventory`
)

//var invSidecarRexp = regexp.MustCompile(`inventory\.json\.(\w+)`)

// Inventory represents contents of an OCFL Object's inventory.json file
type Inventory struct {
	ID               string                `json:"id"`
	Type             string                `json:"type"`
	DigestAlgorithm  string                `json:"digestAlgorithm"`
	Head             string                `json:"head"`
	ContentDirectory string                `json:"contentDirectory,omitempty"`
	Manifest         ContentMap            `json:"manifest"`
	Versions         map[string]Version    `json:"versions"`
	Fixity           map[string]ContentMap `json:"fixity,omitempty"`
}

// Version represent a version entryin inventory.json
type Version struct {
	Created time.Time  `json:"created"`
	State   ContentMap `json:"state"`
	Message string     `json:"message,omitempty"`
	User    User       `json:"user,omitempty"`
}

// User represent a Version's user entry
type User struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}
