package ocfl

import (
	"fmt"
	"os"
	"path/filepath"
)

// Stage Represents is staging area for creating new Object Versions
type Stage struct {
	Version
	Path   string
	object *Object
}

// OpenFile
func (stage *Stage) OpenFile(lPath LPath) (*os.File, error) {
	realPath := filepath.Join(stage.Path, lPath.RelPath())
	stage.State[`_`] = append(stage.State[`_`], lPath)
	return os.OpenFile(realPath, os.O_CREATE|os.O_WRONLY, 0644)
}

func (stage *Stage) Stat(lPath LPath) string {
	for sum, files := range stage.State {
		for i := range files {
			if files[i] == lPath {
				return sum
			}
		}
	}
	return ``
}

func (stage *Stage) Rename(src LPath, dst LPath) error {
	// var err error
	if dstSum := stage.Stat(dst); dstSum != `` {
		return fmt.Errorf(`Already exists: %s`, dst)
	}
	if srcSum := stage.Stat(src); srcSum != `` {
		for i, f := range stage.State[srcSum] {
			if f == src {
				stage.State[srcSum][i] = dst
				return nil
			}
		}
	}
	return fmt.Errorf(`Not found: %s`, src)
}

func (stage *Stage) Remove(lPath LPath) error {
	// var err error
	if sum := stage.Stat(lPath); sum != `` {
		var newFiles []LPath
		for _, f := range stage.State[sum] {
			if f != lPath {
				newFiles = append(newFiles, f)
			}
		}
		stage.State[sum] = newFiles
		return nil
	}
	return fmt.Errorf(`Not found: %s`, lPath)
}
