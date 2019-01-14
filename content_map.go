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

// Add adds a Digest->Path map to the ContentMap. Returns an error if path is already present.
func (cm *ContentMap) Add(digest Digest, path Path) error {
	if *cm == nil {
		*cm = ContentMap{}
	}
	if cm.GetDigest(path) != `` {
		return fmt.Errorf(`already exists: %s`, path)
	}
	if _, ok := (*cm)[digest]; ok {
		if _, ok := (*cm)[digest][path]; !ok {
			(*cm)[digest][path] = true
		} else {
			panic(`ContentMap.GetDigest() is broken`)
		}
	} else {
		(*cm)[digest] = map[Path]bool{path: true}
	}
	return nil
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

// Remove removes path from the ContentMap and returns the digest. Returns error if path is not found
func (cm *ContentMap) Remove(path Path) (Digest, error) {
	digest := cm.GetDigest(path)
	if digest == `` {
		return ``, fmt.Errorf(`not found: %s`, path)
	}
	delete((*cm)[digest], path)
	if len((*cm)[digest]) == 0 {
		delete(*cm, digest)
	}
	return digest, nil
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
