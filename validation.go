package ocfl

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"github.com/srerickson/ocfl/namaste"
)

var versionFormats = map[string]*regexp.Regexp{
	`padded`:   regexp.MustCompile(`v0\d+`),
	`unpadded`: regexp.MustCompile(`v[1-9]\d*`),
}

type Validator struct {
	root     string
	critical []error
	warning  []error
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
	if err := namaste.MatchTypePatternError(path, namasteObjectTValue); err != nil {
		v.addCritical(err)
		return err
	}
	inv, err := v.validateInventory(inventoryFileName)
	if err != nil {
		return err
	}

	var existingVersions []string
	if files, err := ioutil.ReadDir(path); err != nil {
		v.addCritical(err)
	} else {

		for _, f := range files {
			if !f.IsDir() {
				continue
			}
			if style := getVersionFormat(f.Name()); style != `` {
				if styleUsed != versionFormat {
					v.addCritical(fmt.Errorf(`inconsistent version directory style`))
				}
				v.validateObjectVersion(f.Name())
			}
		}

	}
	if len(v.critical) > 0 {
		return v.critical[0]
	}
	return nil
}

func (v *Validator) inventoryIsComplete(inv *Inventory) bool {

	// Must have ID
	if inv.ID == `` {
		v.addCritical(fmt.Errorf(`missing inventory ID`))
	}

	// Validate Version Names in Inventory
	var invVersions []string
	versionFormat := ``
	for verName := range inv.Versions {
		if format := getVersionFormat(verName); format != `` {
			if versionFormat == `` {
				versionFormat = format
			} else if versionFormat != format {
				v.addCritical(fmt.Errorf(`inconsistent version directory format`))
			}
			v.validateObjectVersion(verName)
		}
		invVersions := append(invVersions, verName)
	}

	return true
}

func (v *Validator) inventoryDigests(inv *Inventory) error {
	// Fixity
	for alg, manifest := range inv.Fixity {
		if err := v.validateManifest(manifest, alg); err != nil {
			return err
		}
	}
	// Manifest
	return v.validateManifest(inv.Manifest, inv.DigestAlgorithm)

}

func (v *Validator) validateInventory(name string) (*Inventory, error) {
	i, err := ReadInventory(filepath.Join(v.root, name))
	if err != nil {
		v.addCritical(err)
		return nil, err
	}
	return i, err
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

func (v *Validator) validateObjectVersion(version string) error {
	if v.root == `` {
		return errors.New(`Cannot validate object version: object path not set`)
	}
	inventoryPath := filepath.Join(v.root, version, inventoryFileName)
	if _, err := v.validateInventory(inventoryPath); err != nil {
		return err
	}

	return nil
}

func getVersionFormat(name string) string {
	for style, re := range versionFormats {
		if re.MatchString(name) {
			return style
		}
	}
	return ``
}
