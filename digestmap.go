package ocfl

import (
	"fmt"
	"io/fs"
	"iter"
	"maps"
	"path"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// DigestMap maps digests to file paths.
type DigestMap map[string][]string

// AllPaths returns a sorted slice of all path names in the DigestMap.
func (m DigestMap) AllPaths() []string {
	pths := make([]string, 0, m.NumPaths())
	for _, paths := range m {
		pths = append(pths, paths...)
	}
	sort.Strings(pths)
	return pths
}

// Clone returns a copy of dm
func (m DigestMap) Clone() DigestMap {
	newM := maps.Clone(m)
	for d, p := range newM {
		newM[d] = slices.Clone(p)
	}
	return newM
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

// DigestFor returns the digest for path p or an empty string if the digest is
// not present.
func (m DigestMap) DigestFor(p string) string {
	if p == "" {
		return ""
	}
	for d, pths := range m {
		if slices.Contains(pths, p) {
			return d
		}
	}
	return ""
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
	m1PathMap := m1Norm.PathMap()
	m2PathMap := m2Norm.PathMap()
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
	if err := validPaths(merged.AllPaths()); err != nil {
		return nil, err
	}
	return merged, nil
}

// Mutate applies each path mutation function to paths for each digest in m. If
// the mutations remove all paths for the digest, the digest key is deleted from
// m. Mutate may make m invalid.
func (m DigestMap) Mutate(fns ...PathMutation) {
	for digest := range m {
		for _, fn := range fns {
			m[digest] = fn(m[digest])
		}
		if len(m[digest]) == 0 {
			delete(m, digest)
		}
	}
}

// Normalize checks if m is valid and returns a normalized copy (with lowercase
// digests) and sorted paths.
func (m DigestMap) Normalize() (norm DigestMap, err error) {
	if err := m.Valid(); err != nil {
		return nil, err
	}
	norm = make(DigestMap, len(m))
	for digest, paths := range m {
		normDig := normalizeDigest(digest)
		normPaths := slices.Clone(paths)
		slices.Sort(normPaths)
		norm[normDig] = normPaths
	}
	return
}

// NumPaths returns the number of paths in m
func (m DigestMap) NumPaths() int {
	var l int
	for _, paths := range m {
		l += len(paths)
	}
	return l
}

// PathMap returns a PathMap with m's paths and corresponding digests. The
// returned PathMap may be invalid.
func (m DigestMap) PathMap() PathMap {
	paths := make(PathMap, m.NumPaths())
	maps.Insert(paths, m.Paths())
	return paths
}

// Paths is an iterator that yields path/digest pairs in m. The order paths are
// yielded is not defined.
func (m DigestMap) Paths() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for d, paths := range m {
			for _, p := range paths {
				if !yield(p, d) {
					return
				}
			}
		}
	}
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
	return validPaths(m.AllPaths())
}

// hasDigestCase returns two booleans indicating if any digests in m
// include lowercase and uppercase characters, respectively.
func (m DigestMap) hasDigestCase() (hasLower bool, hasUpper bool) {
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

// validDigests return a MaptDigestConflictErr if m includes two versions of the
// same digest (i.e., upper case and lower case hex values).
func (m DigestMap) validDigests() error {
	hasLower, hasUpper := m.hasDigestCase()
	if !hasLower || !hasUpper {
		// if m's digests are exclusively uppercase or lowercase then they must
		// be valid
		return nil
	}
	norms := make(map[string]bool, len(m))
	for d := range m {
		norm := normalizeDigest(d)
		if norms[norm] {
			return &MapDigestConflictErr{Digest: d}
		}
		norms[norm] = true
	}
	return nil
}

// validPaths checks paths are valid and consistent: it returns
// *MapPathInvalidErr or *MapPathConflictErr if not.
func validPaths(paths []string) error {
	for _, p := range paths {
		if !validPath(p) {
			return &MapPathInvalidErr{Path: p}
		}
	}
	// check for path conflicts. sort paths and check that each is
	// distinct from an not treated as a directory by the following
	// path
	if !slices.IsSorted(paths) {
		slices.Sort(paths)
	}
	n := len(paths)
	if n <= 1 {
		return nil
	}
	for i, p := range paths[:n-1] {
		next := paths[i+1]
		if p == next || strings.HasPrefix(next, p+"/") {
			return &MapPathConflictErr{Path: p}
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

// SortedPaths is an iterator that yields the path/digest pairs in pm in sorted
// order (by pathname).
func (pm PathMap) SortedPaths() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		paths := slices.Collect(maps.Keys(pm))
		slices.Sort(paths)
		for _, p := range paths {
			if !yield(p, pm[p]) {
				return
			}
		}
	}
}

// PathMutation is used with [DigestMap.Mutate] to change paths names
// in a DigestMap
type PathMutation func(oldPaths []string) (newPaths []string)

// RenamePaths returns a PathMutation function that renames occurences of src to
// dst. If src matches a full path, it is replaced with dst. If src matches a
// directory (including '.'), all occurences of the directory prefix are
// replaced with dst (which may be '.').
func RenamePaths(src, dst string) PathMutation {
	return func(paths []string) []string {
		if src == "." {
			// src is root: dst is new parent directory for all paths
			for i, p := range paths {
				paths[i] = path.Join(dst, p)
			}
			return paths
		}
		if idx := slices.Index(paths, src); idx >= 0 {
			// src is a file: rename to dst
			paths[idx] = dst
			return paths
		}
		// at least one path is in a directory named src
		for i, p := range paths {
			if suffix, found := strings.CutPrefix(p, src+"/"); found {
				// src is a directory: move its contents into dir
				paths[i] = path.Join(dst, suffix)
			}
		}
		return paths
	}
}

// RemovePath returns a PathMutation that removes name
func RemovePath(name string) PathMutation {
	return func(paths []string) []string {
		if idx := slices.Index(paths, name); idx >= 0 {
			return slices.Delete(paths, idx, idx+1)
		}
		return paths
	}
}
