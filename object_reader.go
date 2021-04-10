package ocfl

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/srerickson/checksum"
)

const (
	ocflVersion           = "1.0"
	objectDeclaration     = `ocfl_object_` + ocflVersion
	objectDeclarationFile = `0=ocfl_object_` + ocflVersion
	inventoryFile         = `inventory.json`
)

// ObjectReader represents a readable OCFL Object
type ObjectReader struct {
	root      fs.FS      // root fs
	inventory *Inventory // inventory.json
}

// NewObjectReader returns a new ObjectReader with loaded inventory.
// An error is returned only if the inventory cannot be unmarshaled
func NewObjectReader(root fs.FS) (*ObjectReader, error) {
	obj := &ObjectReader{root: root}
	err := obj.readDeclaration()
	if err != nil {
		return nil, err
	}
	obj.inventory, err = obj.readInventory(`.`)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (obj *ObjectReader) sidecarFile() string {
	return inventoryFile + "." + obj.inventory.DigestAlgorithm
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
	defer f.Close()
	decl, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	if string(decl) != objectDeclaration+"\n" {
		return fmt.Errorf(`version declaration invalid: %w`, &ErrE007)
	}
	return nil
}

// reads inventory and calculates checksum
func (obj *ObjectReader) readInventory(dir string) (*Inventory, error) {

	path := filepath.Join(dir, inventoryFile)
	file, err := obj.root.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, &ErrE034
		}
		return nil, err
	}
	defer file.Close()
	if obj.inventory != nil {
		return ReadInventoryChecksum(file, obj.inventory.DigestAlgorithm)
	}
	// we don't know the digest algorithm, so we read inventory
	// and get checksum in two reads.
	inv, err := ReadInventory(file)
	if err != nil {
		return nil, err
	}
	inv.checksum, err = obj.inventoryChecksum("", inv.DigestAlgorithm)
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func (obj *ObjectReader) inventoryChecksum(dir string, alg string) ([]byte, error) {
	newH, err := newHash(alg)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, inventoryFile)
	file, err := obj.root.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, &ErrE034
		}
		return nil, err
	}
	defer file.Close()
	checksum := newH()
	io.Copy(checksum, file)
	return checksum.Sum(nil), nil
}

func (obj *ObjectReader) readInventorySidecar(dir string) (string, error) {
	path := filepath.Join(dir, obj.sidecarFile())
	file, err := obj.root.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", &ErrE058
		}
		return "", err
	}
	defer file.Close()
	cont, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	sidecar := string(cont)
	offset := strings.Index(string(sidecar), " ")
	if offset < 0 || !digestRegexp.MatchString(sidecar[:offset]) {
		return "", fmt.Errorf("invalid sidecar contents: %s: %w", sidecar, &ErrE061)
	}
	return sidecar[:offset], nil
}

type fsOpenFunc func(name string) (fs.File, error)

func (f fsOpenFunc) Open(name string) (fs.File, error) {
	return f(name)
}

// VersionFS returns an fs.FS representing the logical state of the version
func (obj *ObjectReader) VersionFS(vname string) (fs.FS, error) {
	v, ok := obj.inventory.Versions[vname]
	if !ok {
		return nil, fmt.Errorf(`Version not found: %s`, vname)
	}
	var open fsOpenFunc = func(logicalPath string) (fs.File, error) {
		digest := v.State.GetDigest(logicalPath)
		if digest == "" {
			return nil, fmt.Errorf(`%s: %w`, logicalPath, fs.ErrNotExist)
		}
		realpaths := obj.inventory.Manifest[digest]
		if len(realpaths) == 0 {
			return nil, fmt.Errorf(`no manifest entries files associated with the digest: %s`, digest)
		}
		return obj.root.Open(filepath.FromSlash(realpaths[0]))
	}
	return open, nil
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
		contentDir := filepath.Join(v, obj.inventory.ContentDirectory)
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
