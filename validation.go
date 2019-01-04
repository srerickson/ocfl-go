package ocfl

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"

	"github.com/srerickson/ocfl/namaste"
)

var versionDirStyles = map[string]*regexp.Regexp{
	`padded`:   regexp.MustCompile(`v0\d+`),
	`unpadded`: regexp.MustCompile(`v[1-9]\d*`),
}

type Validator struct {
	root      string
	critical  []error
	warning   []error
	inventory *Inventory
	checksums map[string]string
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

// Validate returns validation results for object
func (v *Validator) ValidateObject(path string) error {
	v.init(path)
	if err := namaste.MatchTypePatternError(path, namasteObjectTValue); err != nil {
		v.addCritical(err)
		return err
	}
	if inv, err := ReadInventory(filepath.Join(path, inventoryFileName)); err != nil {
		v.addCritical(err)
		return err
	} else {
		v.inventory = inv
	}
	if err := v.inventory.Validate(path); err != nil {
		v.addCritical(err)
	}
	if files, err := ioutil.ReadDir(path); err != nil {
		v.addCritical(err)
	} else {
		styleUsed := ``
		for _, f := range files {
			if !f.IsDir() {
				continue
			}
			if style := getVersionDirStyle(f.Name()); style != `` {
				if styleUsed == `` {
					styleUsed = style
				} else if styleUsed != style {
					v.addCritical(fmt.Errorf(`inconsistent version directory style`))
				}
				v.validateObjectVersion(f.Name())
			}
		}
		if styleUsed == `` {
			v.addWarning(fmt.Errorf(`Object has no versions`))
		}
	}
	if len(v.critical) > 0 {
		return v.critical[0]
	}
	return nil
}

func (v *Validator) validateObjectVersion(version string) error {
	if v.root == `` {
		return errors.New(`Cannot validate object version: object path not set`)
	}
	inventoryPath := filepath.Join(v.root, version, inventoryFileName)
	if inv, err := ReadInventory(inventoryPath); err != nil {
		v.addCritical(err)
		return err
	} else {
		if err := inv.Validate(v.root); err != nil {
			v.addCritical(err)
		}
	}

	return nil
}

func getVersionDirStyle(name string) string {
	for style, re := range versionDirStyles {
		if re.MatchString(name) {
			return style
		}
	}
	return ``
}
