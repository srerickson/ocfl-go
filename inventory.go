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
	"sort"
	"strings"
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
	Versions         map[string]*Version   `json:"versions"`
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

func ReadInventory(file io.Reader) (*Inventory, error) {
	inv := &Inventory{}
	decoder := json.NewDecoder(file)

	// The OCFL spec (v1.0) allows uknown fields in some places (in
	// Versions, for examples). Using DisallowUnknownFields() would
	// invalidate some valid objects. Best to leave this disabled.
	// decoder.DisallowUnknownFields()

	err := decoder.Decode(inv)
	if err != nil {
		switch err.(type) {
		case *time.ParseError:
			return nil, fmt.Errorf(`date/time format error in inventory.json: %w`, &ErrE049)
		case *json.UnmarshalTypeError:
			if strings.Contains(err.Error(), `Inventory.head`) {
				return nil, fmt.Errorf(`failed to parse head in inventory.json: %w`, &ErrE040)
			}
			if strings.Contains(err.Error(), `Version.versions.message`) {
				return nil, fmt.Errorf(`failed to parese version message in inventory.json: %w`, &ErrE094)
			}
			// Todo other special cases?
		}
		return nil, fmt.Errorf("%s: %w", err.Error(), &ErrE033)

	}
	return inv, nil
}

func (inv *Inventory) Validate() error {

	// one or more versions are present
	if len(inv.Versions) == 0 {
		return fmt.Errorf(`inventory missing 'versions' field: %w`, &ErrE008)
	}
	// id is present
	if inv.ID == "" {
		return fmt.Errorf(`inventory missing 'id' field: %w`, &ErrE036)
	}
	// type is present
	if inv.ID == "" {
		return fmt.Errorf(`inventory missing 'type' field: %w`, &ErrE036)
	}
	if inv.DigestAlgorithm == "" {
		return fmt.Errorf(`inventory missing 'digestAlgorithm' field: %w`, &ErrE036)
	}
	// head is present
	if inv.Head == "" {
		return fmt.Errorf(`inventory missing 'head' field: %w`, &ErrE036)
	}
	// head is a version
	if inv.Versions[inv.Head] == nil {
		return fmt.Errorf(`inventory 'head' value does not correspond to a version: %w`, &ErrE040)
	}
	// manifest is present
	if inv.Manifest == nil {
		return fmt.Errorf(`inventory missing 'manifest' field: %w`, &ErrE041)
	}

	return nil
}

// returns list of version directories
func (inv *Inventory) VersionDirs() []string {
	dirs := make([]string, 0, len(inv.Versions))
	for v := range inv.Versions {
		dirs = append(dirs, v)
	}
	return dirs
}

// ParseVersion returns the last version number and padding level
// for the inventory. If the inventory version names are inconsistent
// or the version numbers are not 1..n, an error is returned.
func (inv *Inventory) ParseVersions() (int, int, error) {
	if len(inv.Versions) == 0 {
		err := fmt.Errorf(`inventory missing 'versions' field: %w`, &ErrE008)
		return 0, 0, err
	}
	padding := -1
	versions := make([]int, 0, len(inv.Versions))
	// check consistent padding
	for d := range inv.Versions {
		v, p, err := versionParse(d)
		if err != nil {
			return 0, 0, err
		}
		versions = append(versions, v)
		if padding == -1 {
			padding = p
			continue
		}
		if p != padding {
			err := fmt.Errorf(`inconsistent version format: %w`, &ErrE012)
			return 0, 0, err
		}
	}
	// check versions: v1...n
	sort.Sort(sort.IntSlice(versions))
	for i, v := range versions {
		if i+1 != v {
			err := fmt.Errorf(`non-sequential version %d: %w`, v, &ErrE009)
			return 0, 0, err
		}
	}
	return versions[0], padding, nil
}
