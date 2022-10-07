package ocfl

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/pathtree"
)

var (
	ErrInvalidPath = pathtree.ErrInvalidPath
	ErrNotFound    = pathtree.ErrNotFound
	ErrNotDir      = pathtree.ErrNotDir
	ErrNotFile     = pathtree.ErrNotFile
	ErrValueExists = pathtree.ErrValueExists
	ErrNoValue     = errors.New("value not set")
)

// Index is a data structure used to represent and change a 'logical state' for
// an OCFL object version. Logical file paths (as found in an inventory's
// version state) are used to reference IndexItem structs. The structure of the
// index reflects the directory structure of the object state. Both logical
// files and logical directories can be managed using Index. Indexes are
// primarily used to 'stage' content before committing new object versions to a
// storage root.
//
// An Index can be 'backed' by an FS, allowing logical paths to be mapped to
// files in the FS. To create an Index backed by an FS use IndexDir().
//
// Index also provides methods for modifying the logical state (see Remove and
// SetDir).
type Index struct {
	// The FS 'backing' entries in SrcPaths. It may be nil. If set, paths in
	// SrcPaths should be relative to the FS
	FS FS
	//
	// The 'primary' algorithm for items in the index. It may be empty. If set,
	// all index entries should include a digest for this algorithm in Digests.
	Alg digest.Alg

	// root is the root node in index. It must a directory node.
	root *pathtree.Node[*IndexItem]
}

func NewIndex() *Index {
	return &Index{
		root: pathtree.NewDir[*IndexItem](),
	}
}

// IndexItem represents state of logical files in Index. It can track all
// digests associated with the path as well any 'source' paths, such as those
// found in an inventory manifest.
type IndexItem struct {
	// Digests stores multiple digests from different algorithms for the logical
	// path in the Index.
	Digests digest.Set `json:"sum,omitempty"`
	// SrcPaths stores slice of content paths (i.e., manifest entries) associated
	// with the logical path.
	SrcPaths []string `json:"src,omitempty"`
}

type DirEntry = pathtree.DirEntry

func (inf *IndexItem) AddSrc(name string) {
	for _, p := range inf.SrcPaths {
		if p == name {
			return
		}
	}
	inf.SrcPaths = append(inf.SrcPaths, name)
	sort.Strings(inf.SrcPaths)
}

func (inf *IndexItem) HasSrc(names ...string) bool {
	for _, n := range names {
		for _, p := range inf.SrcPaths {
			if p == n {
				return true
			}
		}
	}
	return false
}

func (item *IndexItem) SameContentAs(other *IndexItem) (bool, error) {
	var match, mismatch int
	for alg, sum := range item.Digests {
		otherSum, ok := other.Digests[alg]
		if !ok {
			continue
		}
		if sum == otherSum {
			match++
		} else {
			mismatch++
		}
	}
	if match > 0 && mismatch > 0 {
		return false, errors.New("digests matching gave inconsistent results")
	}
	if mismatch > 0 {
		return false, nil
	}
	if match > 0 {
		return true, nil
	}
	if len(item.SrcPaths) == 0 || len(other.SrcPaths) == 0 {
		return false, errors.New("can't determine equivalence")
	}
	return item.HasSrc(other.SrcPaths...), nil
}

// Get returns the *IndexItem associated with path logical and a boolean
// indicating if path is a directory. The *IndexItem may be nil. An error
// is returned the path is invalid or does exist in the tree.
func (idx Index) Get(logical string) (*IndexItem, bool, error) {
	n, err := idx.root.Get(logical)
	if err != nil {
		return nil, false, err
	}
	return n.Val, n.IsDir(), nil
}

// Set sets the *IndexItem for the file at path logical. If the entry does not
// exist, it is created. If the entry exists, the existing *IndexItem is
// replaced.
func (idx *Index) Set(logical string, info *IndexItem) error {
	if err := pathtree.SetVal(idx.root, logical, info, true); err != nil {
		return err
	}
	return nil
}

// Sub returns the subtree rooted at p, which must be a directory path
func (idx *Index) Sub(p string) (*Index, error) {
	n, err := idx.root.Get(p)
	if err != nil {
		return nil, err
	}
	if !n.IsDir() {
		return nil, ErrNotDir
	}
	return &Index{root: n, FS: idx.FS}, nil
}

// SetDir attaches the tree sub to t at path p. If replace is false and error is
// returned if path p already exists. If path p exists as a file node and
// replace is true, the file node will be converted to a directory node. If the
// additition would cause a cycle in tree, an error is returned
func (idx *Index) SetDir(logical string, sub *Index, replace bool) error {
	if idx.FS != nil && sub.FS != nil && idx.FS != sub.FS {
		return errors.New("cannot attach index from a different fs")
	}
	if logical == "." {
		idx.root = sub.root
		if idx.FS == nil && sub.FS != nil {
			idx.FS = sub.FS
		}
		return nil
	}
	if err := idx.root.Set(logical, sub.root, replace); err != nil {
		return err
	}
	if idx.FS == nil && sub.FS != nil {
		idx.FS = sub.FS
	}
	return nil
}

// ReadDir returns ordered slice of DirEntry for contents of p
func (idx *Index) ReadDir(p string) ([]DirEntry, error) {
	n, err := idx.root.Get(p)
	if err != nil {
		return nil, err
	}
	return n.ReadDir(), nil
}

func (idx *Index) Remove(p string, recursive bool) (*Index, error) {
	n, err := idx.root.Remove(p, recursive)
	idx.root.RemoveEmptyDirs()
	return &Index{root: n}, err
}

type IndexWalkFunc func(name string, isdir bool, val *IndexItem) error

func (idx *Index) Walk(fn IndexWalkFunc) error {
	return pathtree.Walk(idx.root, (pathtree.WalkFunc[*IndexItem])(fn))
}

// Let returns number of files in the Tree
func (idx *Index) Len() int {
	return idx.root.Len()
}

func (idx *Index) StateMap(alg digest.Alg) (*digest.Map, error) {
	m := digest.NewMap()
	walkFn := func(p string, isdir bool, inf *IndexItem) error {
		if isdir {
			return nil
		}
		dig, exists := inf.Digests[alg]
		if !exists {
			return fmt.Errorf("%w: '%s'", ErrNoValue, alg)
		}
		if err := m.Add(dig, p); err != nil {
			return err
		}
		return nil
	}
	if err := idx.Walk(walkFn); err != nil {
		return nil, err
	}
	return m, nil
}

func (idx *Index) ManifestMap(alg digest.Alg) (*digest.Map, error) {
	m := digest.NewMap()
	walkFn := func(p string, isdir bool, inf *IndexItem) error {
		if isdir {
			return nil
		}
		dig, exists := inf.Digests[alg]
		if !exists {
			return fmt.Errorf("%w: '%s'", ErrNoValue, alg)
		}
		for _, src := range inf.SrcPaths {
			if err := m.Add(dig, src); err != nil {
				return err
			}
		}
		return nil
	}
	if err := idx.Walk(walkFn); err != nil {
		return nil, err
	}
	return m, nil
}

func (tree Index) MarshalJSON() ([]byte, error) {
	return json.Marshal(tree.root)
}

func (idx *Index) Diff(next *Index, alg digest.Alg) (IndexDiff, error) {
	return indexNodeDiff(idx.root, next.root, alg)
}

func indexNodeDiff(a, b *pathtree.Node[*IndexItem], alg digest.Alg) (IndexDiff, error) {
	diff := IndexDiff{
		Added:     NewIndex(),
		Removed:   NewIndex(),
		Changed:   NewIndex(),
		Unchanged: NewIndex(),
	}
	if a == nil {
		diff.Added = &Index{root: b}
		return diff, nil
	}
	if b == nil {
		diff.Removed = &Index{root: a}
		return diff, nil
	}
	for nA, chA := range a.Children {
		chB, exists := b.Children[nA]
		if !exists {
			// nA is in a, not b -- removed
			diff.addRemoved(nA, chA)
			continue
		}
		// nA exists in both a and b: it may be a directory in both, a file in
		// both, or a directory in one and a file in the other.
		if chA.IsDir() && chB.IsDir() {
			subdiff, err := indexNodeDiff(chA, chB, alg)
			if err != nil {
				return IndexDiff{}, err
			}
			diff.mergeDiff(nA, subdiff)
			continue
		}
		if !chA.IsDir() && !chB.IsDir() {
			same, err := chA.Val.SameContentAs(chB.Val)
			if err != nil {
				return IndexDiff{}, fmt.Errorf("can't diff index: %w", err)
			}
			if same {
				diff.addUnchanged(nA, chA)
			} else {
				diff.addChanged(nA, chA)
			}
			continue
		}
		if (chA.Children == nil) != (chB.Children == nil) {
			// file on one, directory in the other
			diff.addAdded(nA, chB)
			diff.addRemoved(nA, chA)
		}
	}
	for nB, chB := range b.Children {
		_, exists := a.Children[nB]
		if !exists {
			// nB is in b, not a -- added
			diff.addAdded(nB, chB)
		}
	}
	return diff, nil
}

type IndexDiff struct {
	Added     *Index // exist in second, not first
	Removed   *Index // exist in first, not second
	Changed   *Index // exist in both, changed
	Unchanged *Index // exist in both, unchanged
}

func (diff IndexDiff) Equal() bool {
	if diff.Added.Len() > 0 {
		return false
	}
	if diff.Removed.Len() > 0 {
		return false
	}
	if diff.Changed.Len() > 0 {
		return false
	}
	return true
}

func (diff *IndexDiff) addRemoved(name string, node *pathtree.Node[*IndexItem]) error {
	return diff.Removed.root.Set(name, node, true)
}

func (diff *IndexDiff) addAdded(name string, node *pathtree.Node[*IndexItem]) error {
	return diff.Added.root.Set(name, node, true)
}

func (diff *IndexDiff) addChanged(name string, node *pathtree.Node[*IndexItem]) error {
	return diff.Changed.root.Set(name, node, true)
}

func (diff *IndexDiff) addUnchanged(name string, node *pathtree.Node[*IndexItem]) error {
	return diff.Unchanged.root.Set(name, node, true)
}

func (diff *IndexDiff) mergeDiff(name string, sub IndexDiff) error {
	if err := diff.addAdded(name, sub.Added.root); err != nil {
		return err
	}
	if err := diff.addRemoved(name, sub.Removed.root); err != nil {
		return err
	}
	if err := diff.addChanged(name, sub.Changed.root); err != nil {
		return err
	}
	if err := diff.addUnchanged(name, sub.Changed.root); err != nil {
		return err
	}
	return nil
}

func (idx *Index) SetDirDigests(alg digest.Alg) error {
	return digestDirNode(idx.root, alg)
}

func digestDirNode(node *pathtree.Node[*IndexItem], alg digest.Alg) error {
	if !node.IsDir() {
		return nil
	}
	for _, ch := range node.Children {
		if err := digestDirNode(ch, alg); err != nil {
			return err
		}
	}
	h := alg.New()
	for _, d := range node.ReadDir() {
		ch := node.Children[d.Name()]
		sum, exists := ch.Val.Digests[alg]
		if !exists {
			return fmt.Errorf("missing %s digest value: %w", alg, ErrNoValue)
		}
		if _, err := fmt.Fprintf(h, "%x %s\n", sum, d.Name()); err != nil {
			return err
		}
	}
	if node.Val == nil {
		node.Val = &IndexItem{}
	}
	if node.Val.Digests == nil {
		node.Val.Digests = make(digest.Set)
	}
	node.Val.Digests[alg] = hex.EncodeToString(h.Sum(nil))
	return nil
}
