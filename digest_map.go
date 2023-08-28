package ocfl

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

// DigestMap is a data structure for digest/path mapping used in OCFL inventory
// manifests, version states, and fixity. DigestMaps are immutable.
type DigestMap struct {
	digests map[string][]string // digest -> []paths
}

// NewDigestMap returns a NewDigestMap. The validity of the digests values is
// not checked, so the resulting DigestMap may be invalid.
func NewDigestMap(digests map[string][]string) (DigestMap, error) {
	m := DigestMap{digests: digests}
	if err := m.Valid(); err != nil {
		return DigestMap{}, err
	}
	return m, nil
}

// LenDigests returns the number of digests in the DigestMap
func (m DigestMap) LenDigests() int {
	return len(m.digests)
}

// Lenpaths returns the number of paths in the DigestMap
func (m DigestMap) LenPaths() int {
	var l int
	for _, paths := range m.digests {
		l += len(paths)
	}
	return l
}

// Digests returns a slice of the digest values in the DigestMap. Digest strings
// are not normalized; they may be uppercase, lowercase, or mixed.
func (m DigestMap) Digests() []string {
	ret := make([]string, len(m.digests))
	i := 0
	for d := range m.digests {
		ret[i] = d
		i++
	}
	return ret
}

// Paths returns a sorted slice of all path names in the DigestMap.
func (m DigestMap) Paths() []string {
	pths := make([]string, 0, m.LenPaths())
	for _, paths := range m.digests {
		pths = append(pths, paths...)
	}
	sort.Strings(pths)
	return pths
}

// PathMap returns the DigestMap's contents as a map with path names for keys
// and digests for values. PathMap doesn't check if the same path appears
// twice in the DigestMap.
func (m DigestMap) PathMap() map[string]string {
	paths := make(map[string]string, m.LenPaths())
	for d, ps := range m.digests {
		for _, p := range ps {
			paths[p] = d
		}
	}
	return paths
}

// PathMapValid is like PathMap, except it retuns an error if it encounters
// invalid path names or if the same path appears multiple times.
func (m DigestMap) PathMapValid() (map[string]string, error) {
	paths := make(map[string]string, m.LenPaths())
	for d, ps := range m.digests {
		for _, p := range ps {
			if !validPath(p) {
				return nil, &MapPathInvalidErr{p}
			}
			if _, exists := paths[p]; exists {
				return nil, &MapPathConflictErr{Path: p}
			}
			paths[p] = d
		}
	}
	return paths, nil
}

// HasDigest returns true if d is present in the DigestMap. The digest
// is not normalized, so uppercase and lowercase versions of the
// same digest will not count as equivalent.
func (m DigestMap) HasDigest(dig string) bool {
	return len(m.digests[dig]) > 0
}

// DigestPaths returns the slice of paths associated with digest dig
func (m DigestMap) DigestPaths(dig string) []string {
	return slices.Clone(m.digests[dig])
}

// GetDigest returns the digest for path p or an empty string if the digest is
// not present.ß
func (m DigestMap) GetDigest(p string) string {
	var found string
	m.EachPath(func(pth, dig string) bool {
		if pth == p {
			found = dig
			return false
		}
		return true
	})
	return found
}

// EachPath calls fn for each path in the Map. If fn returns false, iteration
// stops and EachPath returns false.
func (m DigestMap) EachPath(fn func(pth, digest string) bool) bool {
	for d, paths := range m.digests {
		for _, p := range paths {
			if !fn(p, d) {
				return false
			}
		}
	}
	return true
}

// Each calls fn for each digest ßin m.  If fn returns false, iteration stops and
// EachPath returns false.
func (m DigestMap) EachDigest(fn func(digest string, paths []string) bool) bool {
	for d, paths := range m.digests {
		if !fn(d, slices.Clone(paths)) {
			return false
		}
	}
	return true
}

// HasUpperCaseDigest returns true if m includes digest values
// with uppercase characters.
func (m DigestMap) HasUppercaseDigest() bool {
	for digest := range m.digests {
		for _, r := range digest {
			if unicode.IsUpper(r) {
				return true
			}
		}
	}
	return false
}

// HasLowercaseDigest returns true if m includes a digest value
// with lowercase characters.
func (m DigestMap) HasLowercaseDigest() bool {
	for digest := range m.digests {
		for _, r := range digest {
			if unicode.IsLower(r) {
				return true
			}
		}
	}
	return false
}

// Eq returns true if m and the other have the same content: they have the
// same (normalized) digests corresponding to the same set of paths. If either
// map has a digest conflict (same digest appears twice with different case), Eq
// returns false.
func (m DigestMap) Eq(other DigestMap) bool {
	mlen := len(m.digests)
	if mlen != len(other.digests) {
		return false
	}
	if mlen == 0 {
		return true
	}
	var err error
	m, err = m.Normalize()
	if err != nil {
		return false
	}
	other, err = other.Normalize()
	if err != nil {
		return false
	}
	for dig, paths := range m.digests {
		if len(paths) == 0 {
			return false
		}
		otherPaths := other.digests[dig]
		if len(paths) != len(otherPaths) {
			return false
		}
		sort.Strings(paths)
		sort.Strings(otherPaths)
		if slices.Compare(paths, otherPaths) != 0 {
			return false
		}
	}
	return true
}

// Normalize returns a normalized version on m (with lowercase digests). An
// error is returned if m has a digest conflict.
func (m DigestMap) Normalize() (norm DigestMap, err error) {
	if !m.HasUppercaseDigest() {
		return m, nil
	}
	norm.digests = make(map[string][]string, len(m.digests))
	for digest, paths := range m.digests {
		normDig := normalizeDigest(digest)
		if _, exists := norm.digests[normDig]; exists {
			err = &MapDigestConflictErr{Digest: normDig}
			return
		}
		norm.digests[normDig] = paths
	}
	return
}

// Remap builds a new DigestMap from m by applying one or more transorfmation
// functions. The transformation function is called for each digest in m and
// returns a new slice of path names to associate with the digest. If the
// returned slices is empty, the digest will not be included in the resulting
// DigestMap. If the transformation functions result in an invalid DigestMap, an
// error is returned.
func (m DigestMap) Remap(fns ...RemapFunc) (DigestMap, error) {
	digests := maps.Clone(m.digests)
	for _, fn := range fns {
		for digest, origPaths := range digests {
			// don't pass m's internal path slice to the function
			newPaths := fn(digest, slices.Clone(origPaths))
			if len(newPaths) == 0 {
				delete(digests, digest)
				continue
			}
			digests[digest] = newPaths
		}
	}
	newMap := DigestMap{digests: digests}
	if err := newMap.Valid(); err != nil {
		return DigestMap{}, err
	}
	return newMap, nil
}

// Merge returns a new DigestMap constructed by normalizing and merging m1 and
// m2. If a paths has different digests in m1 and m2, the value from m2 is used
// if replace is true, otherwise the value from m1 is kept.
func (m1 DigestMap) Merge(m2 DigestMap, replace bool) (merged DigestMap, err error) {
	m1, err = m1.Normalize()
	if err != nil {
		return
	}
	m2, err = m2.Normalize()
	if err != nil {
		return
	}
	m1PathMap, err := m1.PathMapValid()
	if err != nil {
		return
	}
	m2PathMap, err := m2.PathMapValid()
	if err != nil {
		return
	}
	merged.digests = map[string][]string{}
	for pth, dig := range m1PathMap {
		if dig2, ok := m2PathMap[pth]; ok && dig != dig2 {
			if replace {
				continue // use path's digest from m2
			}
		}
		if !slices.Contains(merged.digests[dig], pth) {
			merged.digests[dig] = append(merged.digests[dig], pth)
		}
	}
	for pth, dig := range m2PathMap {
		if dig2, ok := m1PathMap[pth]; ok && dig != dig2 {
			if !replace {
				continue // use path's digest from m1
			}
		}
		if !slices.Contains(merged.digests[dig], pth) {
			merged.digests[dig] = append(merged.digests[dig], pth)
		}
	}
	// check that paths are consistent
	if err = validPaths(merged.Paths()); err != nil {
		merged.digests = nil
	}
	return
}

func (m DigestMap) MarshalJSON() ([]byte, error) {
	if m.digests == nil {
		return json.Marshal(map[string][]string{})
	}
	if err := m.Valid(); err != nil {
		return nil, err
	}
	return json.Marshal(m.digests)
}

func (m *DigestMap) UnmarshalJSON(b []byte) error {
	if m.digests == nil {
		m.digests = map[string][]string{}
	}
	err := json.Unmarshal(b, &m.digests)
	if err != nil {
		return err
	}
	return nil
}

// Valid returns a non-nil error if m is invalid.
func (m DigestMap) Valid() error {
	if m.digests == nil {
		return nil
	}
	if err := m.validDigests(); err != nil {
		return err
	}
	// empty path slices?
	for d, paths := range m.digests {
		if len(paths) == 0 {
			return fmt.Errorf("no paths for digest %q", d)
		}
	}
	return validPaths(m.Paths())
}

func (m DigestMap) validDigests() error {
	if m.HasLowercaseDigest() && m.HasUppercaseDigest() {
		digests := maps.Keys(m.digests)
		for i, d := range digests {
			digests[i] = normalizeDigest(d)
		}
		sort.Strings(digests)
		for i := 0; i < len(digests)-1; i++ {
			if digests[i] == digests[i+1] {
				return &MapDigestConflictErr{Digest: digests[i]}
			}
		}
	}
	return nil
}

func validPaths(paths []string) error {
	// check paths
	files := map[string]struct{}{}
	dirs := map[string]struct{}{}
	for _, p := range paths {
		if !validPath(p) {
			return &MapPathInvalidErr{p}
		}
		if _, exists := files[p]; exists {
			// path appears more than once
			return &MapPathConflictErr{Path: p}
		}
		files[p] = struct{}{}
		if _, exist := dirs[p]; exist {
			// file previously treated as directory
			return &MapPathConflictErr{p}
		}
		for _, parent := range parentDirs(p) {
			// parent previously treated as file
			if _, exists := files[parent]; exists {
				return &MapPathConflictErr{parent}
			}
			dirs[parent] = struct{}{}
		}
	}
	return nil
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
	p = path.Clean(p)
	names := strings.Split(path.Dir(p), "/")
	parents := make([]string, len(names))
	for i := range names {
		parents[i] = strings.Join(names[:i+1], "/")
	}
	return parents
}

func normalizeDigest(d string) string {
	return strings.ToLower(d)
}

// RemapFunc is a function used to transform a DigestMap
type RemapFunc func(digest string, oldPaths []string) (newPaths []string)

// Rename returns a RemapFunc that renames from to to.
func Rename(from, to string) RemapFunc {
	return func(digest string, paths []string) []string {
		for i, p := range paths {
			if p == from {
				paths[i] = to
			}
			after, found := strings.CutPrefix(p, from+"/")
			if found {
				paths[i] = to + "/" + after
			}
		}
		return paths
	}
}

// Remove returns a RemapFunc that removes name.
func Remove(name string) RemapFunc {
	return func(digest string, paths []string) []string {
		idx := slices.Index(paths, name)
		if idx >= 0 {
			return slices.Delete(paths, idx, idx+1)
		}
		return paths
	}
}
