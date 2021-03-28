package ocfl

import (
	"fmt"
	"io/fs"
)

func (obj *ObjectReader) Validate() error {
	if err := obj.Inventory.Validate(); err != nil {
		return err
	}
	if err := obj.validateRoot(); err != nil {
		return err
	}
	return nil
}

// validateRoot validates the object's root file structure. It checks
// existence of required files and absence of illegal files.
func (obj *ObjectReader) validateRoot() error {
	items, err := fs.ReadDir(obj.root, `.`)
	if err != nil {
		return err
	}
	if err := obj._validateRootFiles(items); err != nil {
		return err
	}
	if err := obj._validateRootDirs(items); err != nil {
		return err
	}
	return nil
}

func (obj *ObjectReader) sidecarFile() string {
	return inventoryFile + "." + obj.DigestAlgorithm
}

// helper for validateRoot
func (obj *ObjectReader) _validateRootFiles(items []fs.DirEntry) error {
	onlyFiles := []string{
		inventoryFile,
		obj.sidecarFile(),
		objectDeclarationFile,
	}
	var files []string // existing files
	for _, f := range items {
		name := f.Name()
		if f.Type().IsRegular() {
			files = append(files, name)
		} else if !f.Type().IsDir() {
			return fmt.Errorf(`irregular file: %s`, name)
		}
	}
	missing := minusStrings(onlyFiles, files)
	extra := minusStrings(files, onlyFiles)
	for _, m := range missing {
		switch m {
		case inventoryFile:
			return &ErrE034
		case obj.sidecarFile():
			return &ErrE058
		case objectDeclarationFile:
			return &ErrE003
		}
	}
	if len(extra) != 0 {
		return &ErrE001
	}
	return nil
}

// helper for validateRoot
func (obj *ObjectReader) _validateRootDirs(items []fs.DirEntry) error {
	var vDirs []string
	for _, d := range items {
		name := d.Name()
		if !d.Type().IsDir() {
			continue
		}
		if name == `extensions` {
			continue
		}
		// everything else should be a version dir
		vDirs = append(vDirs, name)
	}
	// version directories must match keys in inventory
	requiredDirs := obj.Inventory.VersionDirs()
	missing := minusStrings(requiredDirs, vDirs)
	extra := minusStrings(vDirs, requiredDirs)
	if len(missing) != 0 {
		return &ErrE046
	}
	if len(extra) > 0 {
		return &ErrE001
	}
	// version sequence is OK
	if err := versionSeqValid(vDirs); err != nil {
		return err
	}
	return nil
}
