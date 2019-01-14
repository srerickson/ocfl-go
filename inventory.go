package ocfl

import (
	"encoding/json"
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
func NewInventory(id string) *Inventory {
	return &Inventory{
		ID:              id,
		Type:            inventoryType,
		DigestAlgorithm: defaultAlgorithm,
		Versions:        map[string]Version{},
		Manifest:        ContentMap{},
		Fixity:          map[string]ContentMap{},
	}
}

// ReadInventory returns Inventory from json file at path
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

// Fprint prints the inventory to writer as json
func (i *Inventory) Fprint(writer io.Writer) error {
	var j []byte
	var err error
	if j, err = json.Marshal(i); err != nil {
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
