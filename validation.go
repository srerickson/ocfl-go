package ocfl

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/srerickson/ocfl/namaste"
)

type Validator struct {
	root          string
	critical      []error
	warning       []error
	versionFormat string
	// inventory *Inventory
	checksums map[string]string // cache of file -> digest
}

func ValidateObject(path string) error {
	var v Validator
	return v.ValidateObject(path)
}

func (v *Validator) init(root string) {
	*v = Validator{
		root:      root,
		checksums: map[string]string{},
	}
}

func (v *Validator) addCritical(err error) {
	v.critical = append(v.critical, err)
}

func (v *Validator) addWarning(err error) {
	v.warning = append(v.warning, err)
}

// ValidateObject validates OCFL object located at path
func (v *Validator) ValidateObject(path string) error {
	v.init(path)

	// Object Conformance Declaration
	// TODO: Get Version Number to Compare to Inventory
	err := namaste.MatchTypePatternError(path, namasteObjectTValue)
	if err != nil {
		v.addCritical(err)
		return err
	}

	// Validate Inventory
	_, err = v.validateInventory(inventoryFileName)
	if err != nil {
		return err
	}

	// Version Directories
	if files, err := ioutil.ReadDir(path); err != nil {
		v.addCritical(err)
	} else {
		for _, f := range files {
			if !f.IsDir() {
				continue
			}
			if style := versionFormat(f.Name()); style != `` {
				v.validateObjectVersionDir(f.Name())
			}
		}

	}
	if len(v.critical) > 0 {
		return v.critical[0]
	}
	return nil
}

func (v *Validator) validateInventory(name string) (*Inventory, error) {
	inv, err := ReadInventory(filepath.Join(v.root, name))
	if err != nil {
		v.addCritical(err)
		return nil, err
	}

	// Validate Inventory Structure:
	// Must have ID
	if inv.ID == `` {
		v.addCritical(fmt.Errorf(`missing inventory ID: %s`, name))
	}
	if inv.Type != inventoryType {
		v.addCritical(fmt.Errorf(`bad type: %s`, inv.Type))
	}
	if inv.DigestAlgorithm == `` {
		v.addCritical(fmt.Errorf(`missing digestAlgorithm: %s`, name))
	}
	if !stringIn(inv.DigestAlgorithm, digestAlgorithms[:]) {
		v.addCritical(fmt.Errorf(`bad digestAlgorithm: %s`, inv.DigestAlgorithm))
	}
	if inv.Manifest == nil {
		v.addCritical(fmt.Errorf(`missing manifest: %s`, name))
	}
	if inv.Versions == nil {
		v.addCritical(fmt.Errorf(`missing version: %s`, name))
	}

	// Validate Version Names in Inventory
	var versions = inv.versionNames()
	var padding int
	if len(versions) > 0 {
		padding = versionPadding(versions[0])
		for i := range versions {
			n, _ := versionGen(i+1, padding)
			if !stringIn(n, versions) {
				v.addCritical(fmt.Errorf(`inconsistent or missing version names: %s`, name))
				break
			}
		}
	} else {
		v.addWarning(fmt.Errorf(`inventory has no versions: %s`, name))
	}

	// Fixity
	for alg, manifest := range inv.Fixity {
		if err := v.validateManifest(manifest, alg); err != nil {
			return nil, err
		}
	}
	// Manifest
	return inv, v.validateManifest(inv.Manifest, inv.DigestAlgorithm)
}

func (v *Validator) validateManifest(m Manifest, alg string) error {
	for expectedSum, paths := range m {
		for _, path := range paths {
			fullPath := filepath.Join(v.root, string(path))
			info, err := os.Stat(fullPath)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("not a regular file: %s", path)
			}
			gotSum, err := Checksum(alg, fullPath)
			if err != nil {
				return err
			}
			if expectedSum != gotSum {
				return fmt.Errorf("checksum failed for %s", path)
			}
		}
	}
	return nil
}

func (v *Validator) validateObjectVersionDir(version string) error {
	if v.root == `` {
		return errors.New(`cannot validate object version: object path not set`)
	}
	inventoryPath := filepath.Join(version, inventoryFileName)
	if _, err := v.validateInventory(inventoryPath); err != nil {
		return err
	}

	return nil
}
