package digest

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/srerickson/ocfl/internal/pathtree"
)

// Map represents the digest/path mapping used in OCFL inventory manifests and
// version states. Map supports read-only access and validation. To create or
// modify a Map, use [MapMaker].
type Map struct {
	// digests is a map of digest strings to path names, as found in an
	// inventory's inventory manifest of version state. The OCFL spec requires
	// the case of digest strings to be preserved-- it's an error when digest
	// strings don't match exactly. Normalizing digests in map keys here would
	// cause invalid inventories to pass validation. Normalization needs to be
	// checked separately during validation.
	digests map[string][]string
}

// AllDigests returns a slice of all digest keys in the Map. Digest strings are
// not normalized; they may be uppercase, lowercase, or mixed.
func (m Map) AllDigests() []string {
	ret := make([]string, len(m.digests))
	i := 0
	for d := range m.digests {
		ret[i] = d
		i++
	}
	return ret
}

// AllPaths returns a mapping between all files and their digests from Map.
// AllPaths will panic if the same path appears twice in the Map or if there is
// an invalid path name in the Map.
func (m Map) AllPaths() map[string]string {
	files, err := m.allPathDigests()
	if err != nil {
		panic(err)
	}
	return files
}

// allPathDigests returns a lookup table of all paths. It checks that all paths are valid
// paths and that they appear only once. It doesn't check collisions with
// directory and path names.
func (m Map) allPathDigests() (map[string]string, error) {
	var leng int
	for _, paths := range m.digests {
		leng += len(paths)
	}
	files := make(map[string]string, leng)
	for d, paths := range m.digests {
		for _, p := range paths {
			if !validPath(p) {
				return nil, &MapPathInvalidErr{p}
			}
			if _, exists := files[p]; exists {
				return nil, &MapPathConflictErr{Path: p}
			}
			files[p] = d
		}
	}
	return files, nil
}

func (m Map) HasUppercaseDigests() bool {
	for digest := range m.digests {
		for _, r := range digest {
			if unicode.IsUpper(r) {
				return true
			}
		}
	}
	return false
}

// Eq returns true if m and the other Map have the same content: they have the
// same (normalized) digests corresponding to the same set of paths. If either
// map has a digest conflict (same digest appears twice with different case), Eq
// returns false.
func (m Map) Eq(other Map) bool {
	mlen := len(m.digests)
	if mlen != len(other.digests) {
		return false
	}
	if mlen == 0 {
		return true
	}
	if m.HasUppercaseDigests() {
		var err error
		m, err = m.Normalized()
		if err != nil {
			return false
		}
	}
	if other.HasUppercaseDigests() {
		var err error
		other, err = other.Normalized()
		if err != nil {
			return false
		}
	}
	for dig, paths := range m.digests {
		otherPaths, ok := other.digests[dig]
		if !ok {
			return false
		}
		if len(paths) != len(otherPaths) {
			return false
		}
		if !sort.IsSorted(sort.StringSlice(paths)) {
			sort.Strings(paths)
		}
		if !sort.IsSorted(sort.StringSlice(otherPaths)) {
			sort.Strings(otherPaths)
		}
		for i, p := range paths {
			if p != otherPaths[i] {
				return false
			}
		}
	}
	return true
}

// Normalized returns a copy of the map with normalized (lowercase) digests and
// sorted slice of paths. An error is returned if the same digest appears more
// than once.
func (m Map) Normalized() (Map, error) {
	cp := Map{
		digests: make(map[string][]string, len(m.digests)),
	}
	for digest, paths := range m.digests {
		norm := normalizeDigest(digest)
		if _, exists := cp.digests[norm]; exists {
			return Map{}, &MapDigestConflictErr{Digest: norm}
		}
		normpaths := append(make([]string, 0, len(paths)), m.digests[digest]...)
		sort.Strings(normpaths)
		cp.digests[norm] = normpaths
	}
	return cp, nil
}

// PathTransform returns a new Map with the transform function applied to the path slice
// for each digest. An error is returned if the resuling Map is invalid.
func (m Map) PathTransform(fn func(digest string, paths []string) []string) (Map, error) {
	entries := make(map[string][]string, len(m.digests))
	for digest, paths := range m.digests {
		// don't pass m's internal path slice to the function
		pathcp := append([]string{}, paths...)
		entries[digest] = fn(digest, pathcp)
	}
	newMap := Map{digests: entries}
	if err := newMap.Valid(); err != nil {
		return Map{}, err
	}
	return newMap, nil
}

// HasDigest returns true if d is present in the Map. The digest
// is not normalized, so uppercase and lowercase versions of the
// same digest will not count as equivalent.
func (m Map) HasDigest(d string) bool {
	_, exists := m.digests[d]
	return exists
}

// DigestPaths returns slice of paths associated with digest dig
func (m Map) DigestPaths(dig string) []string {
	return append(make([]string, 0, len(m.digests[dig])), m.digests[dig]...)
}

// EachPath calls fn for each path in the Map. If fn returns a non-nil error,
// EachPath returns the error and fn is not called again.
func (m Map) EachPath(fn func(name, digest string) error) error {
	for d, paths := range m.digests {
		for _, p := range paths {
			if err := fn(p, d); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetDigest returns the digest for path p
func (m Map) GetDigest(p string) string {
	return m.AllPaths()[p]
}

func (m Map) MarshalJSON() ([]byte, error) {
	if err := m.Valid(); err != nil {
		return nil, err
	}
	if m.digests == nil {
		return json.Marshal(map[string][]string{})
	}
	return json.Marshal(m.digests)
}

func (m *Map) UnmarshalJSON(b []byte) error {
	err := json.Unmarshal(b, &m.digests)
	if err != nil {
		return err
	}
	return nil
}

// NewMapUnsafe is mostly for testing - don't use it. The recommended way to
// create a Map is with MapMaker.
func NewMapUnsafe(d map[string][]string) *Map {
	return &Map{digests: d}
}

// Valid returns a non-nil error if m is invalid.
func (m Map) Valid() error {
	return m.validation()
}

func (m *Map) validation() error {
	files := map[string]string{}
	dirs := map[string]struct{}{}
	norms := map[string]struct{}{}
	for d, paths := range m.digests {
		if len(paths) == 0 {
			return fmt.Errorf("missing path entries for '%s'", d)
		}
		norm := normalizeDigest(d)
		if _, exists := norms[norm]; exists {
			return &MapDigestConflictErr{Digest: norm}
		}
		norms[norm] = struct{}{}
		for _, p := range paths {
			if !validPath(p) {
				return &MapPathInvalidErr{p}
			}
			if _, exists := files[p]; exists {
				// path appears more than once
				return &MapPathConflictErr{Path: p}
			}
			files[p] = d
			if _, exist := dirs[p]; exist {
				// path previously treated as directory
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
	}
	return nil
}

// A MapMaker is used to construct Maps or add new paths to existing digest.
// Maps generated by the MapMaker are always normalized.
type MapMaker struct {
	tree    *pathtree.Node[string]
	digests map[string]struct{} // normalized digests
}

// inititalize MapMaker members if necessary
func (mm *MapMaker) init() {
	if mm.digests == nil {
		mm.digests = map[string]struct{}{}
	}
	if mm.tree == nil {
		mm.tree = pathtree.NewDir[string]()
	}
}

// MapMakerFrom returns a new [MapMaker] that can be used to construct
// and modify a new Map based on an existing Map, m. The existing
// Map is not modified.
func MapMakerFrom(m Map) (*MapMaker, error) {
	mm := &MapMaker{
		tree:    pathtree.NewDir[string](),
		digests: map[string]struct{}{},
	}
	if err := m.validation(); err != nil {
		return nil, fmt.Errorf("digest map has errors: %w", err)
	}
	for pth, dig := range m.AllPaths() {
		norm := normalizeDigest(dig)
		if err := mm.tree.SetFile(pth, norm); err != nil {
			// if the digest map is valid, there should be no errors here.
			panic(fmt.Errorf("this is a bug: %w", err))
		}
		mm.digests[norm] = struct{}{}
	}
	return mm, nil
}

// Add adds the normalized (lowercase) form of the digest d and the path p to
// the MapMaker. If the digest and path are already present, ErrMapPathExists is
// returned. If the path was added with a different digest, or if conflicts with
// another path in the MapMaker, MapPathConflictErr is returned.
func (mm *MapMaker) Add(d, p string) error {
	mm.init()
	if !validPath(p) {
		return &MapPathInvalidErr{p}
	}
	norm := normalizeDigest(d)
	if node, err := mm.tree.Get(p); err == nil {
		if !node.IsDir() && node.Val == norm {
			return ErrMapMakerExists
		}
		return &MapPathConflictErr{Path: p}
	}
	if err := mm.tree.SetFile(p, d); err != nil {
		// an error here indicates that a prefix of p already exists as a file.
		// Ideally we would return the prefix, since that is the source of the
		// conflict.
		return &MapPathConflictErr{Path: p}
	}
	mm.digests[norm] = struct{}{}
	return nil
}

func (mm *MapMaker) AddPaths(d string, paths ...string) error {
	for _, p := range paths {
		if err := mm.Add(d, p); err != nil {
			return err
		}
	}
	return nil
}

// Map returns a point to a new [Map] as constructed or modified by the MapMaker.
func (mm *MapMaker) Map() Map {
	m := Map{digests: map[string][]string{}}
	if mm.tree == nil {
		return m
	}
	pathtree.Walk(mm.tree, func(pth string, node *pathtree.Node[string]) error {
		if node.IsDir() {
			return nil
		}
		d := normalizeDigest(node.Val)
		m.digests[d] = append(m.digests[d], pth)
		return nil
	})
	for _, paths := range m.digests {
		sort.Strings(paths)
	}
	return m
}

// HasDigest returns true if the digest d (or its normalized form)
// exists in the MapMaker. Note this is slightly different than [Map]'s
// method with the same name.
func (mm MapMaker) HasDigest(d string) bool {
	_, exists := mm.digests[normalizeDigest(d)]
	return exists
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
		parents[i] = strings.Join(names[0:i+1], "/")
	}
	return parents
}

func normalizeDigest(d string) string {
	return strings.ToLower(d)
}
