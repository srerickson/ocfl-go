package ocfl

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"path"
	"strings"

	"github.com/srerickson/checksum"
	"github.com/srerickson/ocfl/internal"
)

const (
	ocflVersion           = "1.0"
	objectDeclaration     = `ocfl_object_` + ocflVersion
	objectDeclarationFile = `0=ocfl_object_` + ocflVersion
	inventoryFile         = `inventory.json`
)

// ObjectReader represents a readable OCFL Object
type ObjectReader struct {
	root      fs.FS      // root fs
	inventory *Inventory // inventory.json
	index     internal.PathStore
}

// NewObjectReader returns a new ObjectReader with loaded inventory.
// An error is returned only if:
// 	- OCFL object declaration is missing or invalid.
//  - The inventory is not be present or there was an error loading it
func NewObjectReader(root fs.FS) (*ObjectReader, error) {
	obj := &ObjectReader{root: root}
	err := obj.readDeclaration()
	if err != nil {
		return nil, err
	}
	obj.inventory, err = obj.readInventory(`.`)
	if err != nil {
		//return nil, asValidationErr(err, &ErrE034)
		return nil, err
	}
	obj.index = internal.NewPathStore()

	// add every path from every version to obj.index
	for name, version := range obj.inventory.Versions {
		for digest, paths := range version.State {
			for _, p := range paths {
				path := name + "/" + p
				err := obj.index.Add(path, digest)
				if err != nil {
					if errors.Is(err, internal.ErrPathInvalid) {
						return nil, asValidationErr(err, &ErrE099)
					}
					return nil, asValidationErr(err, &ErrE095)
				}
			}
		}
	}

	return obj, nil
}

// readDeclaration reads and validates the declaration file.
// If an error is returned, it is a ValidationErr
func (obj *ObjectReader) readDeclaration() error {
	f, err := obj.root.Open(objectDeclarationFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &ValidationErr{
				err:  fmt.Errorf(`OCFL version declaration not found: %s`, objectDeclarationFile),
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
			err:  errors.New(`OCFL version declaration has invalid text contents`),
			code: &ErrE007,
		}
	}
	return nil
}

// reads and parses the inventory.json file in dir. If an error is returned
// it may be a ValidationErr if an error occured durind unmarshalling
func (obj *ObjectReader) readInventory(dir string) (*Inventory, error) {
	path := path.Join(dir, inventoryFile)
	file, err := obj.root.Open(path)
	if err != nil {
		return nil, err
	}
	invBytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	file.Close()
	// json schema validation
	err = validateInventoryBytes(invBytes)
	if err != nil {
		return nil, err
	}
	inv, err := ReadInventory(bytes.NewReader(invBytes))
	if err != nil {
		return nil, err
	}
	// additional validation
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
	return inv, nil
}

// reads and validates sidecar. Always returns ValidationErr
func (obj *ObjectReader) readInventorySidecar(dir string, alg string) (string, error) {
	path := path.Join(dir, inventoryFile+"."+alg)
	file, err := obj.root.Open(path)
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

// Content returns DigestMap of all version contents
func (obj *ObjectReader) Content() (DigestMap, error) {
	var content DigestMap
	alg := obj.inventory.DigestAlgorithm
	newH, err := newHash(alg)
	if err != nil {
		return nil, err
	}
	each := func(j checksum.Job, err error) error {
		if err != nil {
			return err
		}
		sum, err := j.SumString(alg)
		if err != nil {
			return err
		}
		return content.Add(sum, j.Path())
	}
	for v := range obj.inventory.Versions {
		contentDir := path.Join(v, obj.inventory.ContentDirectory)
		// contentDir may not exist - that's ok
		err = checksum.Walk(obj.root, contentDir, each, checksum.WithAlg(alg, newH))
		if err != nil {
			walkErr, _ := err.(*checksum.WalkErr)
			if errors.Is(walkErr.WalkDirErr, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
	}
	return content, nil
}
