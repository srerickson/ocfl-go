// Copyright 2019 Seth R. Erickson
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ocfl

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

var (
	// FILEMODE is default FileMode for new files
	FILEMODE os.FileMode = 0644
	// DIRMODE is default FileMode for new directories
	DIRMODE os.FileMode = 0755
)

// Stage represents a staging area for creating new Object Versions
type Stage struct {
	state  ContentMap // next version state
	path   string     // tmp directory for staging new files
	object *Object    // parent object
}

func (stage *Stage) clear() {
	if stage == nil {
		return
	}
	if stage.path != `` {
		os.RemoveAll(stage.path)
		stage.path = ``
	}
	stage.state = nil
}

// Commit creates a new version in the stage's parent object reflecting
// changes made to the stage.
func (stage *Stage) Commit(user User, message string) error {
	if stage.object == nil {
		return errors.New(`stage has no parent object`)
	}
	if stage.state == nil {
		return errors.New(`stage has no state`)
	}
	nextVer, err := stage.object.nextVersion()
	if err != nil {
		return err
	}
	// move tmpdir to version/contents
	verDir := filepath.Join(stage.object.Path, nextVer)
	if err := os.Mkdir(verDir, DIRMODE); err != nil {
		return err
	}
	// if stage has new content, move into version/content dir
	// TODO: if there any empty files in stage dir, delete them
	if stage.path != `` {
		if newFiles, err := ioutil.ReadDir(stage.path); err != nil {
			return err
		} else if len(newFiles) > 0 {
			verContDir := filepath.Join(verDir, `content`)
			if err := os.Rename(stage.path, verContDir); err != nil {
				return err
			}
			walk := func(path string, info os.FileInfo, walkErr error) error {
				if walkErr == nil && info.Mode().IsRegular() {
					alg := stage.object.inventory.DigestAlgorithm
					digest, digestErr := Checksum(alg, path)
					if digestErr != nil {
						return digestErr
					}
					ePath, pathErr := filepath.Rel(stage.object.Path, path)
					if pathErr != nil {
						return pathErr
					}
					vPath, pathErr := filepath.Rel(verContDir, path)
					if pathErr != nil {
						return pathErr
					}
					stage.state.AddReplace(Digest(digest), Path(vPath))
					stage.object.inventory.Manifest.AddDeduplicate(Digest(digest), Path(ePath))
				}
				return walkErr
			}
			filepath.Walk(verContDir, walk)

		}
	}
	newVersion := NewVersion()
	newVersion.State = stage.state.Copy()
	newVersion.User = user
	newVersion.Message = message
	newVersion.Created = time.Now()
	// update inventory
	stage.object.inventory.Versions[nextVer] = newVersion
	stage.object.inventory.Head = nextVer
	// write inventory (twice)
	if err := stage.object.writeInventoryVersion(nextVer); err != nil {
		return err
	}
	return stage.object.writeInventory()
}

// Add adds the file at src to the stage as dst
// - src is copied into the stage's temporary directory
// - src's digest is calculated using parent objects digestAlgorithm
// - An entry (digest->dst) is added to stage state. If dst alread
//   exists, it is removed.
func (stage *Stage) Add(src string, dst string) error {
	return stage.add(src, dst, false)
}

// AddRename is same as Add, except that src is moved rather than
// copied.
func (stage *Stage) AddRename(src string, dst string) error {
	return stage.add(src, dst, true)
}

// add is the business end of Add() and AddRename()
// - src is copied/renamed into the stage's temporary directory
// - src's digest is calculated using parent objects digestAlgorithm
// - file is added to stage.state with AddReplace()
func (stage *Stage) add(src string, dst string, doRename bool) error {
	var dstPath = Path(dst)
	var digest Digest
	var alg = stage.object.inventory.DigestAlgorithm
	if err := dstPath.validate(); err != nil {
		return err
	}
	if err := stage.tempDir(); err != nil {
		return err
	}
	realDst := stage.stagedPath(string(dstPath))
	// Should we remove readlDst if it exists?
	if err := os.MkdirAll(filepath.Dir(realDst), DIRMODE); err != nil {
		return err
	}
	if doRename {
		err := os.Rename(src, realDst)
		if err != nil {
			return err
		}
		sum, err := Checksum(alg, realDst)
		if err != nil {
			return err
		}
		digest = Digest(sum)
	} else {
		srcFile, err := os.Open(src)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		dstFile, err := os.OpenFile(realDst, os.O_CREATE|os.O_RDWR, FILEMODE)
		if err != nil {
			return err
		}
		defer dstFile.Close()
		hash, err := newHash(alg)
		if err != nil {
			return err
		}
		tReader := io.TeeReader(srcFile, hash)
		_, err = io.Copy(dstFile, tReader)
		if err != nil {
			return err
		}
		digest = Digest(hex.EncodeToString(hash.Sum(nil)))
	}
	return stage.state.AddReplace(digest, dstPath)
}

// OpenFile returns a readable and writable *os.File for the given Logical Path.
// If the file has not already been staged (which is the case even if the file
// exists in the current Version State), it is created, along with all parent
// directories. It should not be used to read already committed files: use
// Object.Open() instead.
// func (stage *Stage) OpenFile(lPath string) (*os.File, error) {
// 	if err := stage.tempDir(); err != nil {
// 		return nil, err
// 	}
// 	fullPath := stage.stagedPath(lPath)
// 	err := os.MkdirAll(filepath.Dir(fullPath), DIRMODE)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE, FILEMODE)
// }

// Rename renames files that are staged or that exist in the staged version
func (stage *Stage) Rename(src string, dst string) error {
	var renamedStaged bool
	if stage.isStaged(src) {
		err := os.Rename(stage.stagedPath(src), stage.stagedPath(dst))
		if err != nil {
			return err
		}
		renamedStaged = true
	}
	err := stage.state.Rename(Path(src), Path(dst))
	if err != nil && !renamedStaged {
		return err
	}
	return nil
}

// Remove removes files that are staged or that exist in the staged version
func (stage *Stage) Remove(lPath string) error {
	var removedStaged bool
	if stage.isStaged(lPath) {
		err := os.Remove(stage.stagedPath(lPath))
		if err != nil {
			return err
		}
		removedStaged = true
	}
	_, err := stage.state.Remove(Path(lPath))
	if err != nil && !removedStaged {
		return err
	}
	return nil
}

// stagedPath returns the real path for staged files.
// The file does not necessarily exist
func (stage *Stage) stagedPath(lPath string) string {
	return filepath.Join(stage.path, lPath)
}

// isStaged returns whether the lPath exists as a new/modified file in the stage
func (stage *Stage) isStaged(lPath string) bool {
	_, err := os.Stat(stage.stagedPath(lPath))
	return err == nil
}

func (stage *Stage) tempDir() error {
	var err error
	if stage.object == nil || stage.object.Path == `` {
		return fmt.Errorf(`stage has no parent object`)
	}
	if stage.path == `` {
		stage.path, err = ioutil.TempDir(stage.object.Path, `stage`)
		if err != nil {
			return err
		}
	}
	return nil
}
