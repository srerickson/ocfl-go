package internal

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"path"
	"strings"

	"github.com/srerickson/ocfl/validation"
)

type objectRoot struct{ fs.FS }

// readDeclaration reads and validates the declaration file.
// If an error is returned, it is a ValidationErr
func (root *objectRoot) readDeclaration() error {
	f, err := root.Open(objectDeclarationFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err := fmt.Errorf(`OCFL object declaration not found`)
			return validation.AsVErr(err, &validation.ErrE003)
		}
		return validation.AsVErr(err, &validation.ErrE003)
	}
	defer f.Close()
	decl, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	if string(decl) != objectDeclaration+"\n" {
		err := errors.New(`OCFL object declaration has invalid text contents`)
		return validation.AsVErr(err, &validation.ErrE007)
	}
	return nil
}

// reads and parses the inventory.json file in dir.
func (root *objectRoot) readInventory(dir string, validate bool) (*Inventory, error) {
	path := path.Join(dir, inventoryFile)
	file, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if !validate {
		// inventory digest not set!
		return ReadInventory(file)
	}
	// all validations performed
	invBytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	// json schema validation
	result := validateInventoryBytes(invBytes)
	if !result.Valid() {
		return nil, result
	}
	inv, err := ReadInventory(bytes.NewReader(invBytes))
	if err != nil {
		return nil, err
	}
	// consistency b/w manifest and version states
	err = inv.Validate()
	if err != nil {
		return nil, err
	}
	// digest the inventory
	newH, err := newHash(inv.DigestAlgorithm)
	if err != nil {
		return nil, err
	}
	checksum := newH()
	io.Copy(checksum, bytes.NewReader(invBytes))
	inv.digest = checksum.Sum(nil)
	sidecar, err := root.readInventorySidecar(dir, inv.DigestAlgorithm)
	if err != nil {
		return nil, err
	}
	if hex.EncodeToString(inv.digest) != sidecar {
		err := fmt.Errorf(`inventory checksum validation failed for version %s`, dir)
		return nil, validation.AsVErr(err, &validation.ErrE034)
	}
	return inv, nil
}

// reads and validates sidecar. Always returns ValidationErr
func (root *objectRoot) readInventorySidecar(dir string, alg string) (string, error) {
	path := path.Join(dir, inventoryFile+"."+alg)
	file, err := root.Open(path)
	if err != nil {
		return "", validation.AsVErr(err, &validation.ErrE058)
	}
	defer file.Close()
	cont, err := io.ReadAll(file)
	if err != nil {
		return "", validation.AsVErr(err, &validation.ErrE058)
	}
	sidecar := string(cont)
	offset := strings.Index(string(sidecar), " ")
	if offset < 0 || !digestRegexp.MatchString(sidecar[:offset]) {
		err := fmt.Errorf("invalid sidecar contents: %s", sidecar)
		return "", validation.AsVErr(err, &validation.ErrE061)
	}
	return sidecar[:offset], nil
}
