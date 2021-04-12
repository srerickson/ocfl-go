package ocfl

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
	"errors"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

// digests may be hex encoded, lowercase or uppercase
var digestRegexp = regexp.MustCompile("^[0-9a-fA-F]+$")

// DigestConflictErr indicates digest conflict in
// the DigestMap
type DigestConflictErr struct {
	Digest string
}

func (d *DigestConflictErr) Error() string {
	return "duplicate digest: " + string(d.Digest)
}

// DigestInvalidErr indicates the string is not a valid
// representation of a digest
type DigestInvalidErr struct {
	Digest string
}

func (d *DigestInvalidErr) Error() string {
	return "invalid digest: " + string(d.Digest)
}

// PathConflictErr a path conflic in the DigestMap
type PathConflictErr struct {
	Path string
}

func (p *PathConflictErr) Error() string {
	return "duplicate Path: " + string(p.Path)
}

// PathInvalidErr indicates an invalid path
type PathInvalidErr struct {
	Path string
}

func (p *PathInvalidErr) Error() string {
	return "invalid Path: " + string(p.Path)
}

// DigestMap is a data structure for Content-Addressable-Storage.
// It abstracs the functionality of the Manifest, Version State, and
// Fixity fields in the OCFL object Inventory
type DigestMap map[string][]string

// Add adds a digest->path map to the ContentMap. Returns an error if path is already present.
func (dm *DigestMap) Add(digest string, path string) error {
	if !validPath(path) {
		return &PathInvalidErr{path}
	}
	if dm.GetDigest(path) != `` {
		return &PathConflictErr{path}
	}
	if *dm == nil {
		*dm = DigestMap{}
	}
	(*dm)[digest] = append((*dm)[digest], path)
	return nil
}

func (dm DigestMap) GetDigest(p string) string {
	for d, paths := range dm {
		for _, path := range paths {
			if p == path {
				return d
			}
		}
	}
	return ""
}

// Paths returns a mapping between all files and their digests
// it returns an error if two identical paths are encountered.
func (dm DigestMap) Paths() (map[string]string, error) {
	inv := make(map[string]string)
	for d, paths := range dm {
		for _, p := range paths {
			if _, exists := inv[p]; exists {
				return nil, &PathConflictErr{p}
			}
			inv[p] = d
		}
	}
	return inv, nil
}

func (dm DigestMap) Valid() error {
	_, v := dm.Normalize()
	return v
}

// Normalize returns a new DigestMap with all digests normalized
// (lowercase format). All paths are also validated. If the returned
// error is nill, the returned DigestMap is valid.
func (dm DigestMap) Normalize() (DigestMap, error) {
	if dm == nil {
		return nil, errors.New(`digest map cannot be nil`)
	}
	newDM := make(DigestMap)
	allDirs := make(map[string]bool)
	for d, paths := range dm {
		if !digestRegexp.MatchString(d) {
			return nil, &DigestInvalidErr{d}
		}
		lowerD := strings.ToLower(d)
		if _, exists := newDM[lowerD]; exists {
			return nil, &DigestConflictErr{d}
		}
		newDM[lowerD] = make([]string, len(paths))
		for i, p := range paths {
			if !validPath(p) {
				return nil, &PathInvalidErr{p}
			}
			newDM[lowerD][i] = p
			for _, dir := range parentDirs(p) {
				allDirs[dir] = true
			}
		}
	}
	// no paths should be dirs
	for _, paths := range newDM {
		for _, p := range paths {
			if _, exists := allDirs[p]; exists {
				return nil, &PathConflictErr{p}
			}
		}
	}
	return newDM, nil
}

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
func parentDirs(p string) []string {
	dir := path.Dir(p)
	if dir == "." {
		return nil
	}
	names := strings.Split(dir, "/")
	var ret []string
	for i, n := range names {
		if n == "" {
			continue
		}
		ret = append(ret, strings.Join(names[0:i+1], "/"))
	}
	return ret
}
