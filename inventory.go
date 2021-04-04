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
	"path"
	"strings"
	"time"
)

const (
	inventoryType   = `https://ocfl.io/1.0/spec/#inventory`
	contentDir      = `content`
	digestAlgorithm = "sha512"
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
	if inv.Type == "" {
		return fmt.Errorf(`inventory missing 'type' field: %w`, &ErrE036)
	}
	if inv.DigestAlgorithm == "" {
		return fmt.Errorf(`inventory missing 'digestAlgorithm' field: %w`, &ErrE036)
	}
	// check verions sequence
	if err := versionSeqValid(inv.VersionDirs()); err != nil {
		return err
	}
	// check Head
	if err := inv.validateHead(); err != nil {
		return err
	}
	// check contentDir
	if err := validPath(inv.ContentDirectory); err != nil {
		//return fmt.Errorf("%s: %w", err.Error(), &ErrE00)
		return err
	}
	// manifest is present (can be empty)
	if inv.Manifest == nil {
		return fmt.Errorf(`inventory missing 'manifest' field: %w`, &ErrE041)
	}
	// check manifest path format
	paths, err := inv.Manifest.Paths()
	if err != nil {
		return err
	}
	for path := range paths {
		if err := validPath(path); err != nil {
			return fmt.Errorf("%s: %w", err.Error(), &ErrE099)
		}
	}
	// check version state path format
	for _, v := range inv.Versions {
		paths, err := v.State.Paths()
		if err != nil {
			return fmt.Errorf("%s: %w", err.Error(), &ErrE095)
		}
		for p, _ := range paths {
			if err := validPath(p); err != nil {
				return fmt.Errorf("%s: %w", err.Error(), &ErrE052)
			}
			// check conflics like: "dir", "dir/file"
			if _, present := paths[path.Dir(p)]; present {
				return fmt.Errorf("conflict in %s between %s and %s: %w", v, p, path.Dir(p), &ErrE095)
			}
			// digest capitalization

		}
	}
	return nil
}

// returns list of version directories that should be present in the object root
func (inv *Inventory) VersionDirs() []string {
	dirs := make([]string, 0, len(inv.Versions))
	for v := range inv.Versions {
		dirs = append(dirs, v)
	}
	return dirs
}

func (inv *Inventory) validateHead() error {
	v, _, err := versionParse(inv.Head)
	if err != nil {
		return fmt.Errorf(`inventory 'head' not valid: %w`, &ErrE040)
	}
	if _, ok := inv.Versions[inv.Head]; !ok {
		return fmt.Errorf(`inventory 'head' value does not correspond to a version: %w`, &ErrE040)
	}
	if v != len(inv.Versions) {
		return fmt.Errorf(`inventory 'head' is not the last version: %w`, &ErrE040)
	}
	return nil
}
