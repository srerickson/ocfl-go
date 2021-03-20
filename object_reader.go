package ocfl

import (
	"encoding/json"
	"fmt"
	"io/fs"
)

// ObjectReader represents a readable OCFL Object
type ObjectReader struct {
	root      fs.FS // root fs
	Inventory       // inventory.json
}

// NewObjectReader returns a new ObjectReader with loaded inventory.
// An error is returned only if the inventory cannot be unmarshaled
func NewObjectReader(root fs.FS) (*ObjectReader, error) {
	obj := &ObjectReader{root: root}
	err := obj.loadInventory()
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (obj *ObjectReader) loadInventory() error {
	file, err := obj.root.Open(inventoryFileName)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewDecoder(file).Decode(&obj.Inventory)
}

type versionOpenFunc func(name string) (fs.File, error)

func (f versionOpenFunc) Open(name string) (fs.File, error) {
	return f(name)
}

// VersionFS implements fs.FS for the (logical) contents of the version
func (obj *ObjectReader) VersionFS(vname string) (fs.FS, error) {
	v, ok := obj.Inventory.Versions[vname]
	if !ok {
		return nil, fmt.Errorf(`Version not found: %s`, vname)
	}

	var open versionOpenFunc = func(logicalPath string) (fs.File, error) {
		// TODO: This search should be a Version method
		for digest, paths := range v.State {
			for _, p := range paths {
				if p == logicalPath {
					realpaths := obj.Manifest[digest]
					if len(realpaths) == 0 {
						return nil, fmt.Errorf(`no manifest entries files associated with the digest: %s`, digest)
					}
					return obj.root.Open(realpaths[0])
				}
			}
		}
		return nil, fmt.Errorf(`path not found in version %s: %s`, vname, logicalPath)
	}
	return open, nil
}
