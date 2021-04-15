package ocfl

import (
	"errors"
	"io/fs"
	"path"

	"github.com/srerickson/checksum"
	"github.com/srerickson/ocfl/internal"
)

// ObjectReader represents a readable OCFL Object
type ObjectReader struct {
	root      objectRoot // root fs
	inventory *Inventory // inventory.json
	index     internal.PathStore
}

// NewObjectReader returns a new ObjectReader with loaded inventory.
// An error is returned only if:
// 	- OCFL object declaration is missing or invalid.
//  - The inventory is not be present or there was an error loading it
func NewObjectReader(root fs.FS) (*ObjectReader, error) {
	obj := &ObjectReader{root: objectRoot{root}}
	err := obj.root.readDeclaration()
	if err != nil {
		return nil, err
	}
	// don't validate inventory by default
	obj.inventory, err = obj.root.readInventory(`.`, false)
	if err != nil {
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
