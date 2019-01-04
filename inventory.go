package ocfl

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

// Inventory represents contents of an OCFL Object's inventory.json file
type Inventory struct {
	ID              string             `json:"id"`
	Type            string             `json:"type"`
	DigestAlgorithm string             `json:"digestAlgorithm"`
	Head            string             `json:"head"`
	Manifest        Manifest           `json:"manifest"`
	Versions        map[string]Version `json:"versions"`
	Fixity          Fixity             `json:"fixity"`
}

// Manifest represents manifest elemenf of inventory.json. The manifest key
// is a string representation of a checksum
type Manifest map[string][]EPath

// Version represent a version entryin inventory.json
type Version struct {
	Created time.Time `json:"created"`
	Message string    `json:"message"`
	User    User      `json:"user"`
	State   State     `json:"state"`
}

// User represent a Version's user entry
type User struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// State represents a Version's state element
type State map[string][]LPath

// Fixity represents the Inventory's Fixity element
type Fixity map[string]Manifest

func NewInventory(id string) *Inventory {
	return &Inventory{
		ID:       id,
		Type:     `Object`,
		Versions: map[string]Version{},
		Manifest: Manifest{},
		Fixity:   Fixity{},
	}
}

func (i *Inventory) Validate(rootPath string) error {
	if err := i.validateFixity(rootPath); err != nil {
		return err
	}
	if i.ID == `` {
		return fmt.Errorf(`Missing Inventory ID in %s`, rootPath)
	}
	return i.validateManifest(rootPath)
}

// ValidateManifest returns errors manifest errors (or nil)
func (i *Inventory) validateManifest(rootPath string) error {
	if err := i.Manifest.validate(rootPath, i.DigestAlgorithm); err != nil {
		return err
	}
	return nil
}

// ValidateFixity returns fixity errors (or nil)
func (i *Inventory) validateFixity(rootPath string) error {
	for alg, manifest := range i.Fixity {
		if err := manifest.validate(rootPath, alg); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manifest) validate(rootPath string, alg string) error {
	for expectedSum, paths := range *m {
		for _, path := range paths {
			fullPath := filepath.Join(rootPath, string(path))
			info, err := os.Stat(fullPath)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("Not a regular file: %s", path)
			}
			gotSum, err := Checksum(alg, fullPath)
			if err != nil {
				return err
			}
			if expectedSum != gotSum {
				return fmt.Errorf("Checksum failed for %s", path)
			}
		}
	}
	return nil
}

// ReadInventory returns Inventory
func ReadInventory(path string) (*Inventory, error) {
	var inv Inventory
	var file *os.File
	var invJSON []byte
	var err error
	if file, err = os.Open(path); err != nil {
		return nil, err
	}
	defer file.Close()
	if invJSON, err = ioutil.ReadAll(file); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(invJSON, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

// func (i *Inventory) Write(root string) error {
//
// }

// Sprint prints the inventory to writer as json
func (i *Inventory) Fprint(writer io.Writer) error {
	var j []byte
	var err error
	if j, err = json.Marshal(i); err != nil {
		return err
	}
	_, err = writer.Write(j)
	return err
}
