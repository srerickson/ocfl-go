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
	existing, err := fs.ReadDir(obj.root, `.`)
	if err != nil {
		return err
	}
	sidecarFile := inventoryFile + "." + obj.DigestAlgorithm
	onlyFiles := []string{
		inventoryFile,
		sidecarFile,
		objectDeclarationFile,
	}
	requiredDirs := obj.VersionDirs()
	var files []string // existing
	var dirs []string  // existing
	for _, d := range existing {
		if d.Type().IsRegular() {
			files = append(files, d.Name())
		} else if d.Type().IsDir() {
			dirs = append(dirs, d.Name())
		} else {
			return fmt.Errorf(`irregular file: %s`, d.Name())
		}
	}
	// Files
	missing := minusStrings(onlyFiles, files)
	extra := minusStrings(files, onlyFiles)
	for _, m := range missing {
		switch m {
		case inventoryFile:
			return &ErrE034
		case sidecarFile:
			return &ErrE058
		case objectDeclarationFile:
			return &ErrE003
		}
	}
	if len(extra) != 0 {
		return &ErrE001
	}
	// Directories
	missing = minusStrings(requiredDirs, dirs)
	extra = minusStrings(dirs, requiredDirs)
	if len(missing) != 0 {
		return &ErrE046
	}
	// optional directories
	if len(extra) > 0 && extra[0] != "extensions" {
		return &ErrE001
	}
	// validate version names: padding and v1...n
	_, _, err = obj.Inventory.ParseVersions()
	if err != nil {
		return err
	}
	return nil
}
