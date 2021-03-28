package ocfl

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
)

const (
	ocflVersion           = "1.0"
	objectDeclaration     = `ocfl_object_` + ocflVersion
	objectDeclarationFile = `0=ocfl_object_` + ocflVersion
	inventoryFile         = `inventory.json`
)

// ObjectReader represents a readable OCFL Object
type ObjectReader struct {
	root       fs.FS // root fs
	*Inventory       // inventory.json
}

// NewObjectReader returns a new ObjectReader with loaded inventory.
// An error is returned only if the inventory cannot be unmarshaled
func NewObjectReader(root fs.FS) (*ObjectReader, error) {
	obj := &ObjectReader{root: root}
	err := obj.readInventory(`.`)
	if err != nil {
		return nil, err
	}
	err = obj.readDeclaration()
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// readDeclaration reads and validates the declaration file
func (obj *ObjectReader) readDeclaration() error {
	f, err := obj.root.Open(objectDeclarationFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = fmt.Errorf(`version declaration not found: %w`, &ErrE003)
		}
		return err
	}
	decl, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	if string(decl) != objectDeclaration+"\n" {
		return fmt.Errorf(`version declaration invalid: %w`, &ErrE007)
	}
	return nil
}

func (obj *ObjectReader) readInventory(dir string) error {
	path := filepath.Join(dir, inventoryFile)
	file, err := obj.root.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf(`inventory not f: %w`, &ErrE034)
		}
		return err
	}
	defer file.Close()
	if obj.Inventory, err = ReadInventory(file); err != nil {
		return err
	}
	return nil
}

type fsOpenFunc func(name string) (fs.File, error)

func (f fsOpenFunc) Open(name string) (fs.File, error) {
	return f(name)
}

// VersionFS returns an fs.FS representing the logical state of the version
func (obj *ObjectReader) VersionFS(vname string) (fs.FS, error) {
	v, ok := obj.Inventory.Versions[vname]
	if !ok {
		return nil, fmt.Errorf(`Version not found: %s`, vname)
	}
	var open fsOpenFunc = func(logicalPath string) (fs.File, error) {
		digest := v.State.GetDigest(logicalPath)
		if digest == "" {
			return nil, fmt.Errorf(`%s: %w`, logicalPath, fs.ErrNotExist)
		}
		realpaths := obj.Manifest[digest]
		if len(realpaths) == 0 {
			return nil, fmt.Errorf(`no manifest entries files associated with the digest: %s`, digest)
		}
		return obj.root.Open(filepath.FromSlash(realpaths[0]))
	}
	return open, nil
}
