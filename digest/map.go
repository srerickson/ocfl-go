package digest

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

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// DigestConflictErr indicates same digest found multiple times in the digest map
// (i.e., with different cases)
type DigestConflictErr struct {
	Digest string
}

func (d *DigestConflictErr) Error() string {
	return "digest conflict: " + string(d.Digest)
}

// DigestInvalidErr indicates the string is not a valid representation of a
// digest
type DigestInvalidErr struct {
	Digest string
}

func (d *DigestInvalidErr) Error() string {
	return "invalid digest: " + string(d.Digest)
}

// PathConflictErr indicates same path appears twice in the DigestMap, or the
// path could not be added because it is already present.
type PathConflictErr struct {
	Path string
}

func (p *PathConflictErr) Error() string {
	return "path conflict: " + string(p.Path)
}

// PathInvalidErr indicates an invalid path
type PathInvalidErr struct {
	Path string
}

func (p *PathInvalidErr) Error() string {
	return "invalid path: " + string(p.Path)
}

// BasePathErr indicates that the path is either the base directory of another
// or the other is the base directory of this path.
type BasePathErr struct {
	Path string
}

func (p *BasePathErr) Error() string {
	return "base path error: " + string(p.Path)
}

// Map is a data structure for Content-Addressable-Storage. It abstracs the
// functionality of the Manifest, Version State, and Fixity fields in the OCFL
// object Inventory
//
// A note on digest format: The OCFL spec requires the case of digest strings to
// be preserved. We can't convert all digests to lowercase because digest
// strings must match excactly between the manifest and version state; it's an
// error when they don't match. Automatically converting digests would cause
// invalid inventories to pass validation.
type Map struct {
	// digest -> file paths
	// mixed case digests are possible!
	digests map[string][]string
	// inverses of d: file path-> digest
	files map[string]string
	// index of all parent directories
	dirs map[string]interface{}
	// normalized digest (all lowercase)
	normDigests map[string]interface{}
}

func NewMap() *Map {
	return &Map{
		digests: make(map[string][]string),
	}
}

// Add adds a digest->path to the Map.
// An error is returned if:
//   - digest map is invalid
//   - path string is not valid
//   - path alread exists
//   - path is a basedir for existing path, or
//     existing path is basedir for path
//   - digest is empty string
//   - digest doesn't exist but normalized version does
func (dm *Map) Add(digest string, p string) error {
	if !validPath(p) {
		return &PathInvalidErr{p}
	}
	if dm.isDirty() {
		if err := dm.init(); err != nil {
			return fmt.Errorf("digest map has error: %w", err)
		}
	}
	norm := normalizeDigest(digest)
	_, digestExists := dm.digests[digest]
	_, normExists := dm.normDigests[norm]
	if !digestExists && normExists {
		// digest doesn't exist but another form does
		err := &DigestConflictErr{digest}
		return fmt.Errorf("add: %w", err)
	}
	if _, exists := dm.files[p]; exists {
		err := &PathConflictErr{p}
		return fmt.Errorf("add: %w", err)
	}
	if err := dm.addParents(p); err != nil {
		return fmt.Errorf("add: %w", err)
	}
	dm.files[p] = digest
	dm.digests[digest] = append(dm.digests[digest], p)
	dm.normDigests[norm] = nil
	return nil
}

func (dm Map) Copy() *Map {
	m := NewMap()
	for digest, paths := range dm.digests {
		m.digests[digest] = make([]string, 0, len(paths))
		m.digests[digest] = append(m.digests[digest], dm.digests[digest]...)
	}
	return m
}

func (dm Map) GetDigest(p string) string {
	if dm.isDirty() && dm.init() != nil {
		return ""
	}
	return dm.files[p]
}

func (dm Map) EachPath(fn func(name, digest string) error) error {
	for d, paths := range dm.digests {
		for _, p := range paths {
			if err := fn(p, d); err != nil {
				return err
			}
		}
	}
	return nil
}

func (dm Map) AllDigests() map[string]interface{} {
	// return a copy of dm.digests
	ret := map[string]interface{}{}
	for d := range dm.digests {
		ret[d] = nil
	}
	return ret
}

func (dm Map) DigestExists(d string) bool {
	_, exists := dm.digests[d]
	return exists
}

// AllPaths returns a mapping between all files and their digests
func (dm *Map) AllPaths() map[string]string {
	// return a copy of dm.files
	if dm == nil || (dm.isDirty() && dm.init() != nil) {
		return nil
	}
	ret := map[string]string{}
	for f, d := range dm.files {
		ret[f] = d
	}
	return ret
}

// DigestPaths returns slice of paths associated with digest dig
func (dm Map) DigestPaths(dig string) []string {
	return append(make([]string, 0, len(dm.digests[dig])), dm.digests[dig]...)
}

func (dm *Map) Valid() error {
	dm.setDirty()
	return dm.init()
}

// init regenerates files, dirs, and normDigests from current digests and
// returns any errors
func (dm *Map) init() error {
	dm.files = map[string]string{}
	dm.dirs = map[string]interface{}{}
	dm.normDigests = map[string]interface{}{}
	for d, paths := range dm.digests {
		norm := normalizeDigest(d)
		if _, exists := dm.normDigests[norm]; exists {
			dm.setDirty()
			return &DigestConflictErr{d}
		}
		dm.normDigests[norm] = true
		for _, p := range paths {
			if _, exists := dm.files[p]; exists {
				dm.setDirty()
				return &PathConflictErr{p}
			}
			dm.files[p] = d
			if err := dm.addParents(p); err != nil {
				dm.setDirty()
				return err
			}
		}
	}
	return nil
}

func (dm *Map) setDirty() {
	dm.files = nil
	dm.dirs = nil
	dm.normDigests = nil
}

func (dm *Map) isDirty() bool {
	return dm.files == nil || dm.dirs == nil || dm.normDigests == nil
}

func (dm *Map) addParents(file string) error {
	parents, err := parentDirs(file)
	if err != nil {
		return err
	}
	if dm.isDirty() {
		if err := dm.init(); err != nil {
			return err
		}
	}
	// check that file path does exist as a directory
	if _, exists := dm.dirs[file]; exists {
		return &BasePathErr{file}
	}
	// check that parents don't exist as files
	for _, p := range parents {
		if _, exists := dm.files[p]; exists {
			return &BasePathErr{file}
		}
	}
	for _, p := range parents {
		if _, exists := dm.dirs[p]; !exists {
			dm.dirs[p] = nil
		}
	}
	return nil
}

// func (dm *Map) Merge(dm2 *Map) error {
// 	if err := dm.Valid(); err != nil {
// 		return fmt.Errorf("merge: %w", err)
// 	}
// 	if err := dm2.Valid(); err != nil {
// 		return err
// 	}
// 	for p, d := range dm2.files {
// 		err := dm.Add(d, p)
// 		if err != nil {
// 			var existsErr *PathConflictErr
// 			if errors.As(err, &existsErr) && dm.GetDigest(p) == d {
// 				// not a problem if p exists with same digest
// 				continue
// 			}
// 			return err
// 		}
// 	}
// 	return nil
// }

// validPath returns
func validPath(p string) bool {
	// fs.ValidPath is nearly perfect for OCFL
	if p == "." {
		return false
	}
	return fs.ValidPath(p)
}

// parentDirs returns a slice of paths for each parent of p.
// "a/b/c/d" -> ["a","a/b","a/b/c"]
func parentDirs(p string) ([]string, error) {
	if !validPath(p) {
		return nil, &PathInvalidErr{p}
	}
	p = path.Clean(p)
	names := strings.Split(path.Dir(p), "/")
	ret := make([]string, len(names))
	for i := range names {
		ret[i] = strings.Join(names[0:i+1], "/")
	}
	return ret, nil
}

func normalizeDigest(d string) string {
	return strings.ToLower(d)
}

func (dm *Map) UnmarshalJSON(b []byte) error {
	err := json.Unmarshal(b, &dm.digests)
	if err != nil {
		return err
	}
	return nil
}

func (m Map) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.digests)
}
