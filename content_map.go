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
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ContentMap is a data structure for Content-Addressable-Storage.
// It abstracs the functionality of the Manifest, Version State, and
// Fixity fields in the OCFL object Inventory
type ContentMap map[Digest]map[Path]bool

// Digest is a string representation of a checksum
type Digest string

// Path is a relative file path
type Path string

// DigestPath is a Digest/Path pair, used by Iterate()
type DigestPath struct {
	Digest Digest
	Path   Path
}

//
// ContentMap Functions
//

// Exists returns true if digest/path pair exist in ContentMap
func (cm ContentMap) Exists(digest Digest, path Path) bool {
	if _, ok := cm[digest]; ok {
		if _, ok := cm[digest][path]; ok {
			return true
		}
	}
	return false
}

// GetDigest returns the digest of path in the Content Map.
// Returns an empty digest if not found
func (cm ContentMap) GetDigest(path Path) Digest {
	for digest := range cm {
		if _, ok := cm[digest][path]; ok {
			return digest
		}

	}
	return ``
}

// Digests returns a slice of the digests in the ContentMap
func (cm ContentMap) Digests() []Digest {
	var digests []Digest
	for digest := range cm {
		digests = append(digests, digest)
	}
	return digests
}

// DigestPaths returns slice of Paths with the given digest
func (cm ContentMap) DigestPaths(digest Digest) []Path {
	var paths []Path
	for path := range cm[digest] {
		paths = append(paths, path)
	}
	return paths
}

// Len returns total number of Paths in the ContentMap
func (cm ContentMap) Len() int {
	var size int
	for digest := range cm {
		size += len(cm[digest])
	}
	return size
}

// insert inserts digest->path without checking for path duplication
func (cm *ContentMap) insert(digest Digest, path Path) {
	if *cm == nil {
		*cm = ContentMap{}
	}
	if _, ok := (*cm)[digest]; ok {
		if _, ok := (*cm)[digest][path]; !ok {
			(*cm)[digest][path] = true
		}
	} else {
		(*cm)[digest] = map[Path]bool{path: true}
	}
}

// Add adds a Digest->Path map to the ContentMap. Returns an error if path is already present.
func (cm *ContentMap) Add(digest Digest, path Path) error {
	if cm.GetDigest(path) != `` {
		return fmt.Errorf(`already exists: %s`, path)
	}
	cm.insert(digest, path)
	return nil
}

// AddReplace adds a Digest->Path map, removing previously existing path if necessary
func (cm *ContentMap) AddReplace(digest Digest, path Path) {
	if prev := cm.GetDigest(path); prev != `` {
		cm.delete(prev, path)
	}
	cm.insert(digest, path)
}

// Rename renames src path to dst. Returns an error if dst already exists or src is not found
func (cm *ContentMap) Rename(src Path, dst Path) error {
	if cm.GetDigest(dst) != `` {
		return fmt.Errorf(`already exists: %s`, dst)
	}
	srcDigest := cm.GetDigest(src)
	if srcDigest == `` {
		return fmt.Errorf(`not found: %s`, src)
	}
	delete((*cm)[srcDigest], src)
	(*cm)[srcDigest][dst] = true
	return nil
}

// delete deletes digest->path pair without error check
func (cm *ContentMap) delete(digest Digest, path Path) {
	delete((*cm)[digest], path)
	if len((*cm)[digest]) == 0 {
		delete(*cm, digest)
	}
}

// Remove removes path from the ContentMap and returns the digest. Returns error if path is not found
func (cm *ContentMap) Remove(path Path) (Digest, error) {
	digest := cm.GetDigest(path)
	if digest == `` {
		return ``, fmt.Errorf(`not found: %s`, path)
	}
	cm.delete(digest, path)
	return digest, nil
}

// Iterate returns a channel of DigestPaths in the ContentMap
func (cm ContentMap) Iterate() chan DigestPath {
	ret := make(chan DigestPath)
	go func() {
		for digest := range cm {
			for path := range cm[digest] {
				ret <- DigestPath{digest, path}
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

// UnmarshalJSON implements the Unmarshaler interface for ContentMap.
func (cm *ContentMap) UnmarshalJSON(jsonData []byte) error {
	var tmpMap map[Digest][]Path
	if err := json.Unmarshal(jsonData, &tmpMap); err != nil {
		return err
	}
	*cm = ContentMap{}
	for digest, files := range tmpMap {
		if err := digest.Validate(); err != nil {
			return err
		}
		for i := range files {
			cm.Add(digest, files[i])
		}
	}
	return nil
}

// MarshalJSON implements the Marshaler interface for Path
func (cm ContentMap) MarshalJSON() ([]byte, error) {
	var tmpMap = map[Digest][]Path{}
	for digest := range cm {
		// TODO: why do I have to do this here?
		if err := digest.Validate(); err != nil {
			return nil, err
		}
		for path := range cm[digest] {
			if _, ok := tmpMap[digest]; ok {
				tmpMap[digest] = append(tmpMap[digest], path)
				// sort the array FIXME: only done for testing.
				sort.Slice(tmpMap[digest], func(i int, j int) bool {
					return tmpMap[digest][i] < tmpMap[digest][j]
				})
			} else {
				tmpMap[digest] = []Path{path}
			}
		}
	}
	return json.Marshal(tmpMap)
}

//
// Path Functions
//

// Validate validates the Path value
func (path Path) Validate() error {
	cleanPath := filepath.Clean(string(path))
	if filepath.IsAbs(cleanPath) {
		return fmt.Errorf(`path must be relative: %s`, path)
	}
	if strings.HasPrefix(cleanPath, `..`) {
		return fmt.Errorf(`path is out of scope: %s`, path)
	}
	return nil
}

// UnmarshalJSON implements the Unmarshaler interface for Path.
// It also converts directory separator from slash to system format
func (path *Path) UnmarshalJSON(jsonData []byte) error {
	var tmpPath string
	json.Unmarshal(jsonData, &tmpPath)
	*path = Path(filepath.FromSlash(tmpPath))
	return path.Validate()
}

// MarshalJSON implements the Marshaler interface for Path
func (path Path) MarshalJSON() ([]byte, error) {
	if err := path.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(filepath.ToSlash(string(path)))
}

//
// Digest Functions
//

// Validate validates the Digest value
func (digest Digest) Validate() error {
	if _, err := hex.DecodeString(string(digest)); err != nil {
		return err
	}
	return nil
}

// UnmarshalJSON implements the Unmarshaler interface for Digest
func (digest *Digest) UnmarshalJSON(jsonData []byte) error {
	json.Unmarshal(jsonData, (*string)(digest))
	return digest.Validate()
}

// MarshalJSON implements the Marshaler interface for Digest
func (digest Digest) MarshalJSON() ([]byte, error) {
	if err := digest.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal((string)(digest))
}
