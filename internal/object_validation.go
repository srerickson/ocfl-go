package internal

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"github.com/srerickson/checksum/delta"
)

func (obj *ObjectReader) Validate() *ValidationResult {
	result := &ValidationResult{}
	var err error
	obj.inventory, err = obj.root.readInventory(`.`, true)
	if err != nil {
		return result.AddFatal(err, nil)
	}
	if err := obj.validateRoot(); err != nil {
		return result.AddFatal(err, nil)
	}
	for v := range obj.inventory.Versions {
		err := obj.validateVersionDir(v)
		if err != nil {
			return result.AddFatal(err, nil)
		}
	}
	if err := obj.validateContent(); err != nil {
		return result.AddFatal(err, nil)
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
	match := dirMatch{
		ReqFiles: []string{
			inventoryFile,
			obj.inventory.SidecarFile(),
			objectDeclarationFile,
		},
		ReqDirs: obj.inventory.VersionDirs(),
		OptDirs: []string{"extensions"},
	}
	err = match.Match(items)
	if err != nil {
		if errors.Is(err, errDirMatchMissingFile) {
			if strings.Contains(err.Error(), objectDeclarationFile) {
				return &ValidationErr{&ErrE003, err}
			}
			if strings.Contains(err.Error(), obj.inventory.SidecarFile()) {
				return &ValidationErr{&ErrE058, err}
			}
			if strings.Contains(err.Error(), inventoryFile) {
				return &ValidationErr{&ErrE034, err}
			}
		}
		if errors.Is(err, errDirMatchInvalidFile) {
			return &ValidationErr{&ErrE001, err}
		}
		if errors.Is(err, errDirMatchMissingDir) {
			return &ValidationErr{&ErrE046, err}
		}
		if errors.Is(err, errDirMatchInvalidDir) {
			return &ValidationErr{&ErrE001, err}
		}
		return err
	}
	// err = versionSeqValid(obj.inventory.VersionDirs())
	// if err != nil {
	// 	return err
	// }
	err = obj.validateExtensionsDir()
	if err != nil {
		return err
	}
	return nil
}

func (obj *ObjectReader) validateVersionDir(v string) error {
	items, err := fs.ReadDir(obj.root, v)
	if err != nil {
		return err
	}
	match := dirMatch{
		FileRegexp: regexp.MustCompile(`^inventory\.json(\.[a-z0-9]+)?$`),
		DirRegexp:  regexp.MustCompile(`^.*$`),
	}
	err = match.Match(items)
	if err != nil {
		return &ValidationErr{code: &ErrE015, err: err}
	}
	var hasInventory bool
	for _, i := range items {
		if i.Type().IsRegular() && i.Name() == inventoryFile {
			hasInventory = true
		}
	}
	if hasInventory {
		inv, err := obj.root.readInventory(v, true)
		if err != nil {
			return err
		}
		if obj.inventory.Head == v {
			// if this is the HEAD version, root inventory should match this inventory
			if !bytes.Equal(obj.inventory.digest, inv.digest) {
				return &ValidationErr{
					err:  fmt.Errorf(`root inventory doesn't match inventory for %s`, v),
					code: &ErrE064,
				}
			}
		}
		return nil
	}
	// WARN no inventory
	return nil
}

func (obj *ObjectReader) validateContent() error {
	content, err := obj.Content()
	if err != nil {
		return err
	}
	// path -> digest
	allFiles, err := content.Paths()
	if err != nil {
		return err
	}
	// file and digests in content but not in manifest?
	manifest, err := obj.inventory.Manifest.Normalize()
	if err != nil {
		return err
	}
	paths, err := manifest.Paths()
	if err != nil {
		return err
	}
	changes := delta.New(paths, allFiles)

	if len(changes.Same()) != len(allFiles) || len(changes.Same()) != len(manifest) {
		err := &ContentDiffErr{
			Added:    changes.Added(),
			Removed:  changes.Removed(),
			Modified: changes.Modified(),
		}
		err.RenamedFrom, err.RenamedTo = changes.Renamed()
		if len(err.Modified) != 0 {
			return &ValidationErr{err: err, code: &ErrE092}
		}
		return &ValidationErr{
			err:  fmt.Errorf("content includes files not in manifest"),
			code: &ErrE023,
		}
	}
	// TODO E024 - empty directories
	return nil
}

func (obj *ObjectReader) validateExtensionsDir() error {
	items, err := fs.ReadDir(obj.root, "extensions")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	match := dirMatch{
		// only contain directories
		DirRegexp: regexp.MustCompile("^.*$"),
	}
	err = match.Match(items)
	if err != nil {
		return &ValidationErr{err: err, code: &ErrE067}
	}
	return nil
}
