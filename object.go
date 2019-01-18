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
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/srerickson/ocfl/namaste"
)

const (
	namasteObjectTValue = `ocfl_object_1.0`
	namasteObjectFValue = "ocfl_object_\n"
	inventoryFileName   = `inventory.json`
)

// Object represents an OCFL Object
type Object struct {
	Path      string
	inventory *Inventory
	stage     *Stage
}

// InitObject creates a new OCFL object at path with given ID.
func InitObject(path string, id string) (Object, error) {
	var o Object
	absPath, err := filepath.Abs(path)
	if err != nil {
		return o, err
	}
	if err := os.MkdirAll(absPath, DIRMODE); err != nil {
		return o, err
	}
	o = Object{Path: absPath}
	o.inventory = NewInventory(id)
	if err := namaste.SetType(o.Path, namasteObjectTValue, namasteObjectFValue); err != nil {
		return o, err
	}
	return o, o.writeInventory()
}

// GetObject returns and *Object representing the OCFL object stored
// at path
func GetObject(path string) (*Object, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	err = namaste.MatchTypePatternError(absPath, namasteObjectTValue)
	if err != nil {
		return nil, err
	}
	inv, err := ReadInventory(filepath.Join(absPath, inventoryFileName))
	if err != nil {
		return nil, err
	}
	return &Object{
		Path:      absPath,
		inventory: inv,
	}, nil
}

// Open opens a file in the most recent version using its logical path.
func (o *Object) Open(path string) (*os.File, error) {
	if o.inventory == nil {
		return nil, errors.New(`object has no inventory`)
	}
	vName := o.inventory.Head
	if vName == `` {
		return nil, errors.New(`object has no versions`)
	}
	version, ok := o.inventory.Versions[vName]
	if !ok {
		return nil, fmt.Errorf(`version not found: %s`, vName)
	}
	digest := version.State.GetDigest(Path(path))
	if digest == `` {
		return nil, fmt.Errorf(`file not found: %s`, path)
	}
	ePaths := o.inventory.Manifest.DigestPaths(digest)
	if len(ePaths) > 0 {
		return os.Open(filepath.Join(o.Path, string(ePaths[0])))
	}
	return nil, fmt.Errorf(`no path in manifest for digest: %s`, digest)
}

// Iterate returns channel of DigestPath in latest version
func (o *Object) Iterate() chan DigestPath {
	if o.inventory == nil {
		return nil
	}
	vName := o.inventory.Head
	if vName == `` {
		return nil
	}
	version, ok := o.inventory.Versions[vName]
	if !ok {
		return nil
	}
	return version.State.Iterate()
}

func (o *Object) writeInventoryVersion(ver string) error {
	invPath := filepath.Clean(filepath.Join(o.Path, ver, inventoryFileName))
	file, err := os.OpenFile(invPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if err := o.inventory.Fprint(file); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	digest, err := Checksum(`sha512`, invPath)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(invPath+`.sha512`), []byte(digest), 0644)
}

func (o *Object) writeInventory() error {
	return o.writeInventoryVersion(``)
}

// NewStage returns a new Stage for creating new Object versions.
func (o *Object) NewStage() (*Stage, error) {
	if o.stage != nil {
		o.stage.clear()
	} else {
		o.stage = &Stage{
			object: o,
		}
	}
	inv, err := ReadInventory(filepath.Join(o.Path, inventoryFileName))
	if err != nil {
		return nil, err
	}
	if headVer, ok := inv.Versions[inv.Head]; !ok {
		o.stage.state = ContentMap{}
	} else {
		o.stage.state = headVer.State.Copy()
	}
	return o.stage, nil
}

func (o *Object) nextVersion() (string, error) {
	if o.inventory.Head == `` {
		return version1, nil
	}
	return nextVersionLike(o.inventory.Head)
}
