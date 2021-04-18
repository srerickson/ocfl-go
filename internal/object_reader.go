package internal

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/srerickson/checksum"
)

// ObjectReader represents a readable OCFL Object
type ObjectReader struct {
	root      objectRoot // root fs
	inventory *Inventory // inventory.json
	logical   fs.FS
}

// NewObjectReader returns a new ObjectReader with loaded inventory.
// An error is returned only if:
// 	- OCFL object declaration is missing or invalid.
//  - The inventory is not be present or there was an error loading it
func NewObjectReader(root fs.FS) (*ObjectReader, error) {
	if root == nil {
		return nil, errors.New("cannot read nil FS")
	}
	obj := &ObjectReader{root: objectRoot{root}}
	err := obj.root.readDeclaration()
	if err != nil {
		return nil, err
	}
	// don't validate inventory by default
	obj.inventory, err = obj.root.readInventory(`.`, false)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, asValidationErr(err, &ErrE063)
		}
		return nil, err
	}
	return obj, nil
}

func (obj *ObjectReader) LogicalFS() (fs.FS, error) {
	files := make(map[string]string)
	// add every path from every version to obj.index
	for vname, version := range obj.inventory.Versions {
		paths, err := version.State.Paths()
		if err != nil {
			return nil, asValidationErr(err, &ErrE095)
		}
		for p, digest := range paths {
			targets := obj.inventory.Manifest[digest]
			if len(targets) == 0 {
				return nil, fmt.Errorf("empty path list for digest: %s", digest)
			}
			files[vname+"/"+p] = targets[0]
		}
	}
	logical, err := NewAliasFS(obj.root, files)
	if err != nil {
		if errors.Is(err, ErrPathInvalid) {
			return nil, asValidationErr(err, &ErrE099)
		}
		return nil, asValidationErr(err, &ErrE095)
	}
	return logical, nil
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
