package ocfl

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/srerickson/ocfl/namaste"
)

const (
	namasteObjectTValue = `ocfl_object_1.0`
	namasteObjectFValue = "ocfl_object_\n"
	inventoryFileName   = `inventory.json`
)

// Object represents an OCFL Object
type Object struct {
	Path      string
	inventory *Inventory
	stage     *Stage
}

// InitObject creates a new OCFL object at path with given ID.
func InitObject(path string, id string) (Object, error) {
	var o Object
	if absPath, err := filepath.Abs(path); err != nil {
		return o, err
	} else {
		if err := os.MkdirAll(absPath, 0755); err != nil {
			return o, err
		}
		o = Object{Path: absPath}
	}
	o.inventory = NewInventory(id)
	if err := namaste.SetType(o.Path, namasteObjectTValue, namasteObjectFValue); err != nil {
		return o, err
	}
	return o, o.writeInventory()
}

func (o *Object) writeInventoryVersion(ver string) error {
	invPath := filepath.Clean(filepath.Join(o.Path, ver, inventoryFileName))
	file, err := os.OpenFile(invPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if err := o.inventory.Fprint(file); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	digest, err := Checksum(`sha512`, invPath)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(invPath+`.sha512`), []byte(digest), 0644)
}

func (o *Object) writeInventory() error {
	return o.writeInventoryVersion(``)
}

// NewStage returns a new Stage for creating new Object versions.
func (o *Object) NewStage() (*Stage, error) {
	if o.stage != nil {
		os.RemoveAll(o.stage.Path)
	} else {
		o.stage = &Stage{
			object: o,
		}
	}
	if dir, err := ioutil.TempDir(o.Path, `stage`); err != nil {
		return nil, err
	} else {
		o.stage.Path = dir
	}
	inv, err := ReadInventory(filepath.Join(o.Path, inventoryFileName))
	if err != nil {
		return nil, err
	}
	if headVer, ok := inv.Versions[inv.Head]; !ok {
		o.stage.Version = Version{
			State: State{},
		}
	} else {
		o.stage.Version = headVer
	}
	return o.stage, nil
}

func (o *Object) nextVersion() (string, error) {
	if o.inventory.Head == `` {
		return `v1`, nil
	}
	return `v1`, nil
}

func (o *Object) getExistingPath(digest string) (string, error) {
	if o.inventory == nil {
		return ``, errors.New(`object has no inventory`)
	}
	files, ok := o.inventory.Manifest[digest]
	if !ok || len(files) == 0 {
		return ``, fmt.Errorf(`not found: %s`, digest)
	}
	return string(files[0]), nil
}
