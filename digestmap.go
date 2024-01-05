package ocfl

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/exp/maps"
)

// DigestMap maps digests to file paths.
type DigestMap map[string][]string

// NumPaths returns the number of paths in the m
func (m DigestMap) NumPaths() int {
	var l int
	for _, paths := range m {
		l += len(paths)
	}
	return l
}

// Digests returns a slice of the digest values in the DigestMap. Digest strings
// are not normalized; they may be uppercase, lowercase, or mixed.
func (m DigestMap) Digests() []string {
	ret := make([]string, len(m))
	i := 0
	for d := range m {
		ret[i] = d
		i++
	}
	return ret
}

// Paths returns a sorted slice of all path names in the DigestMap.
func (m DigestMap) Paths() []string {
	pths := make([]string, 0, m.NumPaths())
	for _, paths := range m {
		pths = append(pths, paths...)
	}
	sort.Strings(pths)
	return pths
}

// PathMap returns the DigestMap's contents as a map with path names for keys
// and digests for values. PathMap doesn't check if the same path appears
// twice in the DigestMap.
func (m DigestMap) PathMap() PathMap {
	paths := make(PathMap, m.NumPaths())
	for d, ps := range m {
		for _, p := range ps {
			paths[p] = d
		}
	}
	return paths
}

// PathMapValid is like PathMap, except it returns an error if it encounters
// invalid path names or if the same path appears multiple times.
func (m DigestMap) PathMapValid() (PathMap, error) {
	paths := make(PathMap, m.NumPaths())
	for d, ps := range m {
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

// GetDigest returns the digest for path p or an empty string if the digest is
// not present.
func (m DigestMap) GetDigest(p string) string {
	for d, pths := range m {
		if slices.Contains(pths, p) {
			return d
		}
	}
	return ""
}

// EachPath calls fn for each path in the Map. If fn returns false, iteration
// stops and EachPath returns false.
func (m DigestMap) EachPath(fn func(pth, digest string) bool) bool {
	for d, paths := range m {
		for _, p := range paths {
			if !fn(p, d) {
				return false
			}
		}
	}
	return true
}

// HasDigestCase returns two booleans indicating whether m's digests use
// lowercase and uppercase characters.
func (m DigestMap) HasDigestCase() (hasLower bool, hasUpper bool) {
	for digest := range m {
		for _, r := range digest {
			switch {
			case unicode.IsLower(r):
				hasLower = true
			case unicode.IsUpper(r):
				hasUpper = true
			}
			if hasLower && hasUpper {
				return
			}
		}
	}
	return
}

// Eq returns true if m and the other have the same content: they have the
// same (normalized) digests corresponding to the same set of paths. If either
// map has a digest conflict (same digest appears twice with different case), Eq
// returns false.
func (m DigestMap) Eq(other DigestMap) bool {
	if len(m) != len(other) {
		return false
	}
	if len(m) == 0 {
		return true
	}
	otherNorm, err := other.Normalize()
	if err != nil {
		return false
	}
	for dig, paths := range m {
		if len(paths) == 0 {
			return false
		}
		otherPaths := otherNorm[normalizeDigest(dig)]
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

// Normalize returns a normalized copy on m (with lowercase digests). An
// error is returned if m has a digest conflict.
func (m DigestMap) Normalize() (norm DigestMap, err error) {
	norm = make(DigestMap, len(m))
	for digest, paths := range m {
		normDig := normalizeDigest(digest)
		if _, exists := norm[normDig]; exists {
			err = &MapDigestConflictErr{Digest: normDig}
			return
		}
		norm[normDig] = slices.Clone(paths)
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
	digests := maps.Clone(m)
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
	newMap := DigestMap(digests)
	if err := newMap.Valid(); err != nil {
		return DigestMap{}, err
	}
	return newMap, nil
}

// Merge returns a new DigestMap constructed by normalizing and merging m1 and
// m2. If a paths has different digests in m1 and m2, an error returned unless
// replace is true, in which case the value from m2 is used.
func (m1 DigestMap) Merge(m2 DigestMap, replace bool) (DigestMap, error) {
	m1Norm, err := m1.Normalize()
	if err != nil {
		return nil, err
	}
	m2Norm, err := m2.Normalize()
	if err != nil {
		return nil, err
	}
	m1PathMap, err := m1Norm.PathMapValid()
	if err != nil {
		return nil, err
	}
	m2PathMap, err := m2Norm.PathMapValid()
	if err != nil {
		return nil, err
	}
	merged := DigestMap{}
	for pth, dig := range m1PathMap {
		if dig2, ok := m2PathMap[pth]; ok && dig != dig2 {
			// same path in m1 and m2, but different digests
			if !replace {
				return nil, &MapPathConflictErr{Path: pth}
			}
			// use digest from m2
			dig = dig2
		}
		if !slices.Contains(merged[dig], pth) {
			merged[dig] = append(merged[dig], pth)
		}
	}
	for pth, dig := range m2PathMap {
		if _, exists := m1PathMap[pth]; exists {
			// already merged
			continue
		}
		if !slices.Contains(merged[dig], pth) {
			merged[dig] = append(merged[dig], pth)
		}
	}
	// check that paths are consistent
	if err := validPaths(merged.Paths()); err != nil {
		return nil, err
	}
	return merged, nil
}

// Valid returns a non-nil error if m is invalid.
func (m DigestMap) Valid() error {
	if err := m.validDigests(); err != nil {
		return err
	}
	// empty path slices?
	for d, paths := range m {
		if len(paths) == 0 {
			return fmt.Errorf("no paths for digest %q", d)
		}
	}
	return validPaths(m.Paths())
}

func (m DigestMap) validDigests() error {
	hasLower, hasUpper := m.HasDigestCase()
	if !(hasLower && hasUpper) {
		return nil
	}
	norms := make(map[string]struct{}, len(m))
	for d := range m {
		norm := normalizeDigest(d)
		if _, exists := norms[norm]; exists {
			return &MapDigestConflictErr{Digest: d}
		}
		norms[norm] = struct{}{}
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

// PathMap maps filenames to digest strings.
type PathMap map[string]string

// DigestMap returns a new DigestMap using the pathnames and digests in pm. The
// resulting DigestMap may be invalid if pm includes invalid paths or digests.
func (pm PathMap) DigestMap() DigestMap {
	dm := DigestMap{}
	for pth, dig := range pm {
		dm[dig] = append(dm[dig], pth)
	}
	return dm
}

// DigestMap returns a new DigestMap using the pathnames and digests in pm. If
// the resulting DigestMap is invalid, an error is returned.
func (pm PathMap) DigestMapValid() (DigestMap, error) {
	dm := pm.DigestMap()
	if err := dm.Valid(); err != nil {
		return nil, err
	}
	return dm, nil
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
