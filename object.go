package ocfl

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
)

type objectRoot struct{ fs.FS }

// readDeclaration reads and validates the declaration file.
// If an error is returned, it is a ValidationErr
func (root *objectRoot) readDeclaration() error {
	f, err := root.Open(objectDeclarationFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &ValidationErr{
				err:  fmt.Errorf(`OCFL object declaration not found`),
				code: &ErrE003,
			}
		}
		return &ValidationErr{err: err, code: &ErrE003}
	}
	defer f.Close()
	decl, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	if string(decl) != objectDeclaration+"\n" {
		return &ValidationErr{
			err:  errors.New(`OCFL object declaration has invalid text contents`),
			code: &ErrE007,
		}
	}
	return nil
}

// reads and parses the inventory.json file in dir. If validation is performed
// it may be a validation set
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
	err = validateInventoryBytes(invBytes)
	if err != nil {
		return nil, err
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
		return nil, &ValidationErr{
			err:  fmt.Errorf(`inventory checksum validation failed for version %s`, dir),
			code: &ErrE034,
		}
	}
	return inv, nil
}

// reads and validates sidecar. Always returns ValidationErr
func (root *objectRoot) readInventorySidecar(dir string, alg string) (string, error) {
	path := path.Join(dir, inventoryFile+"."+alg)
	file, err := root.Open(path)
	if err != nil {
		return "", &ValidationErr{
			err:  err,
			code: &ErrE058,
		}
	}
	defer file.Close()
	cont, err := io.ReadAll(file)
	if err != nil {
		return "", &ValidationErr{
			err:  err,
			code: &ErrE058,
		}
	}
	sidecar := string(cont)
	offset := strings.Index(string(sidecar), " ")
	if offset < 0 || !digestRegexp.MatchString(sidecar[:offset]) {
		return "", &ValidationErr{
			err:  fmt.Errorf("invalid sidecar contents: %s", sidecar),
			code: &ErrE061,
		}
	}
	return sidecar[:offset], nil
}
