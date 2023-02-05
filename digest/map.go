package digest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

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
	digests   map[string][]string
	files     map[string]string // inverse of digests for quick access
	validated bool
	err       error // validation error
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
func (m *Map) AllPaths() map[string]string {
	if m.files == nil {
		files, err := m.allPathDigests()
		if err != nil {
			panic(err)
		}
		m.files = files
	}
	return m.files
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
			if _, exists := m.files[p]; exists {
				return nil, &MapPathConflictErr{Path: p}
			}
			files[p] = d
		}
	}
	return files, nil
}

// Copy returns a distinct copy of the Map.
func (m Map) Copy() *Map {
	cp := &Map{
		digests: make(map[string][]string, len(m.digests)),
	}
	for digest, paths := range m.digests {
		cp.digests[digest] = make([]string, 0, len(paths))
		cp.digests[digest] = append(cp.digests[digest], m.digests[digest]...)
	}
	return cp
}

// Normalized returns a copy of the map with normalized (lowercase) digests. An
// error is returned if the same digest appears more than once.
func (m Map) Normalized() (*Map, error) {
	cp := &Map{
		digests: make(map[string][]string, len(m.digests)),
	}
	for digest, paths := range m.digests {
		norm := normalizeDigest(digest)
		if _, exists := cp.digests[norm]; exists {
			return nil, &MapDigestConflictErr{Digest: norm}
		}
		cp.digests[norm] = make([]string, 0, len(paths))
		cp.digests[norm] = append(cp.digests[digest], m.digests[digest]...)
	}
	return cp, nil
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
	if m.files == nil {
		m.files = m.AllPaths()
	}
	return m.files[p]
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
func (m *Map) Valid() error {
	if !m.validated {
		m.err = m.validation()
		m.validated = true
	}
	return m.err
}

func (m *Map) validation() error {
	m.files = map[string]string{}
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
			if _, exists := m.files[p]; exists {
				// path appears more than once
				return &MapPathConflictErr{Path: p}
			}
			m.files[p] = d
			if _, exist := dirs[p]; exist {
				// path previously treated as directory
				return &MapPathConflictErr{p}
			}
			for _, parent := range parentDirs(p) {
				// parent previously treated as file
				if _, exists := m.files[parent]; exists {
					return &MapPathConflictErr{parent}
				}
				dirs[parent] = struct{}{}
			}
		}
	}
	return nil
}

// A MapMaker is used to construct Maps or add new paths to existing digest.
type MapMaker struct {
	tree  *pathtree.Node[string]
	norms map[string]string // normalized digest map
}

// inititalize MapMaker members if necessary
func (mm *MapMaker) init() {
	if mm.norms == nil {
		mm.norms = map[string]string{}
	}
	if mm.tree == nil {
		mm.tree = pathtree.NewDir[string]()
	}
}

// MapMakerFrom returns a new [MapMaker] that can be used to construct
// and modify a new Map based on an existing Map, m. The existing
// Map is not modified.
func MapMakerFrom(m *Map) (*MapMaker, error) {
	mm := &MapMaker{
		tree:  pathtree.NewDir[string](),
		norms: map[string]string{}}

	if err := m.EachPath(func(p, d string) error {
		if !validPath(p) {
			return &MapPathInvalidErr{p}
		}
		norm := normalizeDigest(d)
		prevDigest, exists := mm.norms[norm]
		if exists && d != prevDigest {
			return &MapDigestConflictErr{Digest: norm}
		}
		if _, err := mm.tree.Get(p); err == nil {
			return &MapPathConflictErr{Path: p}
		}
		if err := mm.tree.SetFile(p, d); err != nil {
			return &MapPathConflictErr{Path: p}
		}
		mm.norms[norm] = d
		return nil
	}); err != nil {
		return nil, fmt.Errorf("digest map has errors: %w", err)
	}
	return mm, nil
}

// Add adds digest d and path p to the Map. The digest d may be added using a
// modified form. For example, if the lowercase version of the digest exists and
// the uppercase form is used in Add, the resulting digest map will associate
// path p with the lowercase form. An error is returned if p is already present
// in the MapMaker, or if adding the digest and path would otherwise result in
// an invalid Map. Note, you cannot use Add() to change the digest for a path in
// the MapMaker.
func (mm *MapMaker) Add(d, p string) error {
	mm.init()
	if !validPath(p) {
		return &MapPathInvalidErr{p}
	}
	if d == "" {
		return errors.New("cannot add empty digest")
	}
	norm := normalizeDigest(d)
	prevDigest, digestExists := mm.norms[norm]
	if digestExists {
		// set d to previously added form
		d = prevDigest
	}
	if _, err := mm.tree.Get(p); err == nil {
		// path already exists
		return &MapPathConflictErr{Path: p}
	}
	if err := mm.tree.SetFile(p, d); err != nil {
		// an error here indicates that a prefix of p already exists as a file.
		// Ideally we would return the prefix, since that is the source of the
		// conflict.
		return &MapPathConflictErr{Path: p}
	}
	if !digestExists {
		mm.norms[norm] = d
	}
	return nil
}

// func (mm *MapMaker) GetDigest(p string) string {
// 	if mm.tree == nil || mm.norms == nil {
// 		return ""
// 	}
// 	n, err := mm.tree.Get(p)
// 	if err != nil {
// 		return ""
// 	}
// 	return mm.norms[n.Val]
// }

// Map returns a point to a new [Map] as constructed or modified by the MapMaker.
func (mm *MapMaker) Map() *Map {
	m := &Map{digests: map[string][]string{}}
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
	_, exists := mm.norms[normalizeDigest(d)]
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
