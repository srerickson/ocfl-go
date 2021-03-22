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
	"fmt"
	"path/filepath"
	"strings"
)

// ContentMap is a data structure for Content-Addressable-Storage.
// It abstracs the functionality of the Manifest, Version State, and
// Fixity fields in the OCFL object Inventory
type ContentMap map[string][]string

// File is a Digest/Path pair, used by Iterate()
type File struct {
	Path   string
	Digest string
}

// used only for (un)marshalling
// type jsonPath string

//
// ContentMap Functions
//

// gets the index of path in the []string at cm[digest]
func (cm ContentMap) getIdx(digest string, path string) (int, bool) {
	for i := range cm[digest] {
		if path == cm[digest][i] {
			return i, true
		}
	}
	return -1, false
}

// get digest and index for path
func (cm ContentMap) lookup(path string) (string, int) {
	for digest := range cm {
		if i, ok := cm.getIdx(digest, path); ok {
			return digest, i
		}
	}
	return ``, -1
}

// insert adds digest->path without any checks
func (cm *ContentMap) insert(digest string, path string) {
	if *cm == nil {
		*cm = ContentMap{}
	}
	(*cm)[digest] = append((*cm)[digest], path)
}

// delete deletes digest->path pair without any checks
func (cm *ContentMap) delete(digest string, i int) {
	(*cm)[digest] = append((*cm)[digest][:i], (*cm)[digest][i+1:]...)
	if len((*cm)[digest]) == 0 {
		delete(*cm, digest)
	}
}

// Exists returns true if digest/path pair exist in ContentMap
func (cm ContentMap) Exists(digest string, path string) bool {
	_, ok := cm.getIdx(digest, path)
	return ok
}

// GetDigest returns the digest of path in the Content Map.
// Returns an empty digest if not found
func (cm ContentMap) GetDigest(path string) string {
	digest, _ := cm.lookup(path)
	return digest
}

// Digests returns a slice of the digests in the ContentMap
func (cm ContentMap) Digests() []string {
	var digests = make([]string, 0, len(cm))
	for digest := range cm {
		digests = append(digests, digest)
	}
	return digests
}

// DigestPaths returns slice of Paths with the given digest
func (cm ContentMap) DigestPaths(digest string) []string {
	return cm[digest]
}

// LenDigest returns number of paths associated with digest
func (cm ContentMap) LenDigest(digest string) int {
	return len(cm[digest])
}

// Len returns total number of Paths in the ContentMap
func (cm ContentMap) Len() int {
	var size int
	for digest := range cm {
		size += len(cm[digest])
	}
	return size
}

// Add adds a digest->path map to the ContentMap. Returns an error if path is already present.
func (cm *ContentMap) Add(digest string, path string) error {
	if err := validPath(path); err != nil {
		return err
	}
	if cm.GetDigest(path) != `` {
		return fmt.Errorf(`already exists: %s`, path)
	}
	cm.insert(digest, path)
	return nil
}

// AddReplace adds a Digest->Path map, removing previously existing path if necessary
func (cm *ContentMap) AddReplace(digest string, path string) error {
	if err := validPath(path); err != nil {
		return err
	}
	prev, i := cm.lookup(path)
	if prev != `` {
		cm.delete(prev, i)
	}
	cm.insert(digest, path)
	return nil
}

// AddDeduplicate adds digest/path pair only if no other paths are associated
// with the digest. It returns true if the path was added, false otherwise.
// An error is returned if path is invalid or path is already associated with
// a digest. AddDeduplicate is used to add digests/paths to manifests that
// implement deduplication.
func (cm *ContentMap) AddDeduplicate(digest string, path string) (bool, error) {
	if err := validPath(path); err != nil {
		return false, err
	}
	if cm.GetDigest(path) != `` {
		return false, fmt.Errorf(`already exists: %s`, path)
	}
	if len((*cm)[digest]) > 0 {
		return false, nil
	}
	cm.insert(digest, path)
	return true, nil
}

// Rename renames src path to dst. Returns an error if dst already exists or src is not found
func (cm *ContentMap) Rename(src string, dst string) error {
	if err := validPath(dst); err != nil {
		return err
	}
	if cm.GetDigest(dst) != `` {
		return fmt.Errorf(`already exists: %s`, dst)
	}
	digest, i := cm.lookup(src)
	if digest == `` {
		return fmt.Errorf(`not found: %s`, dst)
	}
	(*cm)[digest][i] = dst
	return nil
}

// Remove removes path from the ContentMap and returns the digest. Returns error if path is not found
func (cm *ContentMap) Remove(path string) (string, error) {
	if err := validPath(path); err != nil {
		return ``, err
	}
	digest, i := cm.lookup(path)
	if digest == `` {
		return ``, fmt.Errorf(`not found: %s`, path)
	}
	cm.delete(digest, i)
	return digest, nil
}

// Iterate returns a channel of DigestPaths in the ContentMap
func (cm ContentMap) Iterate() chan File {
	ret := make(chan File)
	go func() {
		for digest := range cm {
			for i := range cm[digest] {
				ret <- File{
					Digest: digest,
					Path:   cm[digest][i],
				}
			}
		}
		close(ret)
	}()
	return ret
}

// Copy returns a new ContentMap with same content/digest entries
func (cm ContentMap) Copy() ContentMap {
	var newCm ContentMap
	for dp := range cm.Iterate() {
		newCm.insert(dp.Digest, dp.Path)
	}
	return newCm
}

// Subset return wheather cm2 is a subset of cm
func (cm ContentMap) Subset(cm2 ContentMap) bool {
	for dp := range cm2.Iterate() {
		if !cm.Exists(dp.Digest, dp.Path) {
			return false
		}
	}
	return true
}

// EqualTo returns whether cm and cm2 are the same
func (cm ContentMap) EqualTo(cm2 ContentMap) bool {
	return cm.Subset(cm2) && cm2.Subset(cm)
}

// validPath returns
func validPath(path string) error {
	cleanPath := filepath.Clean(path)
	if filepath.IsAbs(cleanPath) {
		return fmt.Errorf(`path must be relative: %s`, cleanPath)
	}
	if strings.HasPrefix(cleanPath, `..`) {
		return fmt.Errorf(`path is out of scope: %s`, cleanPath)
	}
	return nil
}

// Validate validates the Digest value
func validDigest(digest string) error {
	if _, err := hex.DecodeString(string(digest)); err != nil {
		return err
	}
	return nil
}
