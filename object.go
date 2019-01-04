package ocfl

import (
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

func InitObject(path string, id string) (Object, error) {
	o := Object{Path: path}
	o.inventory = NewInventory(id)
	err := namaste.SetType(o.Path, namasteObjectTValue, namasteObjectFValue)
	if err != nil {
		return o, err
	}
	return o, o.WriteInventory()
}

func (o *Object) WriteInventory() error {
	file, err := os.OpenFile(filepath.Join(o.Path, `inventory.json`), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	o.inventory.Fprint(file)
	return nil
}

func (o *Object) NewStage() (*Stage, error) {
	if o.stage != nil {
		os.RemoveAll(o.Path)
	}
	dir, err := ioutil.TempDir(o.Path, `stage`)
	if err != nil {
		return nil, err
	}
	inv, err := ReadInventory(filepath.Join(o.Path, `inventory.json`))
	if err != nil {
		return nil, err
	}
	head, ok := inv.Versions[inv.Head]
	if !ok {
		head = Version{
			State: State{},
		}
	}
	o.stage = &Stage{
		Path:    dir,
		Version: head,
		object:  o,
	}
	// read latest inventory
	return o.stage, nil
}

//
// func (o *Object) Open(logicalPath string, version string) (*os.File, error) {
//
// }
//
func (o *Object) CommitStage() error {
	nextVersion, err := o.nextVersion()
	if err != nil {
		return err
	}
	o.inventory.Versions[nextVersion] = o.stage.Version
	return o.WriteInventory()
}

func (o *Object) nextVersion() (string, error) {
	if o.inventory.Head == `` {
		return `1`, nil
	}
	return `1`, nil
}
