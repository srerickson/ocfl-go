package ocfl

import "time"

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
type Manifest map[Checksum][]ExistingPath

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

// Checksum represents a generic Checksum (for any algorithm)
type Checksum string

// ExistingPath represents an OCFL Existing File Path
type ExistingPath string

// LogicalPath represents an OCFL Logial File Path
type LogicalPath string
