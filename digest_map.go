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
	"fmt"
	"path"
	"regexp"
	"strings"
)

var (
	digestRegexp          = regexp.MustCompile("^[0-9a-fA-F]+$")
	digestLowercaseRegexp = regexp.MustCompile("^[0-9a-f]+$")
	digestUppercaseRegexp = regexp.MustCompile("^[0-9A-F]+$")
)

// DigestMap is a data structure for Content-Addressable-Storage.
// It abstracs the functionality of the Manifest, Version State, and
// Fixity fields in the OCFL object Inventory
type DigestMap map[string][]string

// Add adds a digest->path map to the ContentMap. Returns an error if path is already present.
func (dm *DigestMap) Add(digest string, path string) error {
	if err := validPath(path); err != nil {
		return err
	}
	if dm.GetDigest(path) != `` {
		return fmt.Errorf(`already exists: %s`, path)
	}
	if *dm == nil {
		*dm = DigestMap{}
	}
	if _, ok := (*dm)[digest]; ok {
		(*dm)[digest] = append((*dm)[digest], path)
	} else {
		(*dm)[digest] = []string{path}
	}
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
				return nil, fmt.Errorf(`duplicate path in content map: %s`, p)
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
			return nil, fmt.Errorf(`invalid digests: %s`, d)
		}
		lowerD := strings.ToLower(d)
		if _, exists := newDM[lowerD]; exists {
			return nil, fmt.Errorf(`duplicate digests: %s and %s`, d, lowerD)
		}
		newDM[lowerD] = make([]string, len(paths))
		for i, p := range paths {
			if err := validPath(p); err != nil {
				return nil, err
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
				return nil, fmt.Errorf("path %s also used as a directory", p)
			}
		}
	}
	return newDM, nil
}

// validPath returns
func validPath(p string) error {
	cleanPath := path.Clean(p)
	if p != cleanPath || strings.HasPrefix(p, `.`) {
		return fmt.Errorf(`path includes elements ('.','..','//'): %s`, p)
	}
	if path.IsAbs(cleanPath) {
		return fmt.Errorf(`path must be relative: %s`, cleanPath)
	}
	return nil
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
