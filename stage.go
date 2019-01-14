package ocfl

import (
	"errors"
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
	if stage.object.inventory == nil {
		return errors.New(`stage parent object has no inventory`)
	}
	nextVer, err := stage.object.nextVersion()
	if err != nil {
		return err
	}
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
	stage.object.inventory.Versions[nextVer] = stage.Version
	stage.object.inventory.Head = nextVer

	// write inventory (twice)
	if err := stage.object.writeInventoryVersion(nextVer); err != nil {
		return err
	}
	return stage.object.writeInventory()
}

// OpenFile provides a copy-on-write (COW) interface for the version's files.
func (stage *Stage) OpenFile(lPath string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(stage.existingPath(lPath), flag, perm)
}

// Rename renames files that are staged or exist in the verion
func (stage *Stage) Rename(src string, dst string) error {
	var renamedStaged bool
	if stage.isStaged(src) {
		err := os.Rename(stage.existingPath(src), stage.existingPath(dst))
		if err != nil {
			return err
		}
		renamedStaged = true
	}
	err := stage.Version.State.Rename(Path(src), Path(dst))
	if err != nil && !renamedStaged {
		return err
	}
	return nil
}

// Remove removes files that are staged or exist in the verion
func (stage *Stage) Remove(lPath string) error {
	var removedStaged bool
	if stage.isStaged(lPath) {
		err := os.Remove(stage.existingPath(lPath))
		if err != nil {
			return err
		}
		removedStaged = true
	}
	_, err := stage.Version.State.Remove(Path(lPath))
	if err != nil && !removedStaged {
		return err
	}
	return nil
}

// existingPath gives return the real path from the logical path for a
// staged file. The file does not necessarily exist
func (stage *Stage) existingPath(lPath string) string {
	return filepath.Join(stage.Path, filepath.FromSlash(lPath))
}

// isStaged returns whether the lPath exists as a new/modified file in the stage
func (stage *Stage) isStaged(lPath string) bool {
	_, err := os.Stat(stage.existingPath(lPath))
	return !os.IsNotExist(err)
}
