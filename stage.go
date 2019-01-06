package ocfl

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// Stage represents a staging area for creating new Object Versions
type Stage struct {
	Version
	Path   string
	object *Object
}

func (stage *Stage) Commit() error {
	if stage.object == nil {
		return errors.New(`stage has no parent object`)
	}
	nextVer, err := stage.object.nextVersion()
	if err != nil {
		return err
	}
	fmt.Println(`nextVersion: ` + nextVer)
	// move tmpdir to version/contents
	verDir := filepath.Join(stage.object.Path, nextVer)
	if err := os.Mkdir(verDir, 0755); err != nil {
		return err
	}
	// if stage has new content, move into version/content dir
	if stage.Path != `` {
		if newFiles, err := ioutil.ReadDir(stage.Path); err != nil {
			return err
		} else if len(newFiles) > 0 {
			if err := os.Rename(stage.Path, filepath.Join(verDir, `content`)); err != nil {
				return err
			}
		}
	}

	// update inventory
	//
	stage.object.inventory.Versions[nextVer] = stage.Version

	// write inventory (twice)
	if err := stage.object.writeInventoryVersion(nextVer); err != nil {
		return err
	}
	return stage.object.writeInventory()
}

// OpenFile opens lPath in write-only mode, creating the file if necessary.
// Because the digest is not yet known, it is added to the Version  State using a
// temporary digest key.
func (stage *Stage) OpenFile(lPath LPath) (*os.File, error) {
	_ = stage.Remove(lPath)
	if _, err := stage.Add(lPath, `-`); err != nil {
		return nil, err
	}
	realPath := filepath.Join(stage.Path, lPath.RelPath())
	return os.OpenFile(realPath, os.O_CREATE|os.O_WRONLY, 0644)
}
