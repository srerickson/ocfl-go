package ocfl

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io/fs"
	"regexp"
	"strings"

	"github.com/srerickson/checksum/delta"
)

func (obj *ObjectReader) Validate() error {
	if err := obj.Inventory.Validate(); err != nil {
		return err
	}
	sidecar, err := obj.readInventorySidecar(".")
	if err != nil {
		return err
	}
	check, err := obj.inventoryChecksum(".", obj.Inventory.DigestAlgorithm)
	if err != nil {
		return &ErrE058
	}
	if hex.EncodeToString(check) != sidecar {
		return &ErrE058
	}
	if err := obj.validateRoot(); err != nil {
		return err
	}
	for v := range obj.Inventory.Versions {
		err := obj.validateVersionDir(v)
		if err != nil {
			return err
		}
	}
	if err := obj.validateContent(); err != nil {
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
	match := dirMatch{
		ReqFiles: []string{
			inventoryFile,
			obj.sidecarFile(),
			objectDeclarationFile,
		},
		ReqDirs: obj.Inventory.VersionDirs(),
		OptDirs: []string{"extensions"},
	}
	err = match.Match(items)
	if err != nil {
		if errors.Is(err, errDirMatchMissingFile) {
			if strings.Contains(err.Error(), objectDeclarationFile) {
				return &ErrE003
			}
			if strings.Contains(err.Error(), obj.sidecarFile()) {
				return &ErrE058
			}
			if strings.Contains(err.Error(), inventoryFile) {
				return &ErrE034
			}
		}
		if errors.Is(err, errDirMatchInvalidFile) {
			return &ErrE001
		}
		if errors.Is(err, errDirMatchMissingDir) {
			return &ErrE046
		}
		if errors.Is(err, errDirMatchInvalidDir) {
			return &ErrE001
		}
		return err
	}
	err = versionSeqValid(obj.Inventory.VersionDirs())
	if err != nil {
		return err
	}
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
		ReqFiles: []string{
			inventoryFile,
			obj.sidecarFile(),
		},
		DirRegexp: regexp.MustCompile("^.*$"),
	}
	err = match.Match(items)
	if err != nil {
		if errors.Is(err, errDirMatchMissingFile) {
			if strings.Contains(err.Error(), obj.sidecarFile()) {
				return &ErrE058
			}
			if strings.Contains(err.Error(), inventoryFile) {
				return &ErrE034
			}
		}
		if errors.Is(err, errDirMatchInvalidFile) {
			return &ErrE015
		}
		return err
	}
	inv, err := obj.readInventory(v)
	if err != nil {
		return err
	}
	if obj.Inventory.Head == v {
		// if this is the HEAD version, root inventory should match this inventory
		if !bytes.Equal(obj.Inventory.checksum, inv.checksum) {
			return &ErrE064
		}
	}
	sidecar, err := obj.readInventorySidecar(v)
	if err != nil {
		return err
	}
	if hex.EncodeToString(inv.checksum) != sidecar {
		return &ErrE058
	}
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
	manifest, err := obj.Manifest.Normalize()
	if err != nil {
		return err
	}
	paths, err := manifest.Paths()
	if err != nil {
		return err
	}
	changes := delta.New(paths, allFiles)

	if len(changes.Same()) != len(allFiles) || len(changes.Same()) != len(manifest) {
		mods := changes.Modified()
		if len(mods) != 0 {
			return &ErrE092
		}
		added := changes.Added()
		if len(added) != 0 {
			return &ErrE023
		}
		removed := changes.Removed()
		if len(removed) != 0 {
			return &ErrE023
		}
		old, _ := changes.Renamed()
		if len(old) != 0 {
			return &ErrE023
		}
	}
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
		return &ErrE067
	}
	return nil
}
