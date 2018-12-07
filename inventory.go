package ocfl

import (
	"fmt"
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

// Manifest represents manifest elemenf of inventory.json
type Manifest map[string][]ExistingPath

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
type State map[string][]LogicalPath

// Fixity represents a Version's fmt.Printf("", var)ixity element
type Fixity map[string]Manifest

// ExistingPath represents an OCFL Existing File Path
type ExistingPath string

// LogicalPath represents an OCFL Logial File Path
type LogicalPath string

// ValidateManifest returns errors manifest errors (or nil)
func (i *Inventory) ValidateManifest(rootPath string) error {
	if err := i.Manifest.validate(rootPath, i.DigestAlgorithm); err != nil {
		return err
	}
	return nil
}

// ValidateFixity returns fixity errors (or nil)
func (i *Inventory) ValidateFixity(rootPath string) error {
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
