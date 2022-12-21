package ocfl

import (
	"errors"
	"fmt"

	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/pathtree"
)

var (
	ErrInvalidPath = pathtree.ErrInvalidPath
	ErrNotFound    = pathtree.ErrNotFound
)

// Index represents the logical state of an OCFL object version.
type Index struct {
	node pathtree.Node[IndexItem]
}

func (idx Index) IsDir() bool { return idx.node.IsDir() }

func (idx Index) Val() IndexItem { return idx.node.Val }

func (idx Index) Len() int { return idx.node.Len() }

// Children returns an ordered slice of strings with the names of idx's
// immediate children. If idx is not a directory node, the return value is nil.
func (idx Index) Children() []string {
	if !idx.IsDir() {
		return nil
	}
	entrs := idx.node.DirEntries()
	chld := make([]string, len(entrs))
	for i := range entrs {
		chld[i] = entrs[i].Name()
	}
	return chld
}

// GetVal returns the IndexItem stored in idx for path p along with a
// boolean indicating if path is a directory. An error is returned if no value
// is stored for p or if p is not a valid path.
func (idx Index) GetVal(p string) (IndexItem, bool, error) {
	sub, err := idx.node.Get(p)
	if err != nil {
		return IndexItem{}, false, err
	}
	return sub.Val, sub.IsDir(), nil
}

// ErrSkipDir can be returned in an IndexWalkFunc to prevent Walk from calling
// the walk function for descendants of a directory.
var ErrSkipDir = pathtree.ErrSkipDir

// IndexWalkFunc is a function called by Walk for each path in an Index. The
// path p is relative to the Index receiver for Walk. sub is an Index
// representing the sub-Index at path p.
type IndexWalkFunc func(p string, sub *Index) error

// Walk calls fn for each path in idx, starting with itself.
func (idx *Index) Walk(fn IndexWalkFunc) error {
	wrap := func(name string, n *pathtree.Node[IndexItem]) error {
		return fn(name, &Index{node: *n})
	}
	return pathtree.Walk(&idx.node, wrap)
}

// SetRoot is used internally to set the contents of an Indexß. It shouldn't be
// usable outside this package or its subpackages.
func (idx *Index) SetRoot(root *pathtree.Node[IndexItem]) {
	idx.node = *root
}

// return an empty directory index
func newEmptyIndex() *Index {
	return &Index{node: *pathtree.NewDir[IndexItem]()}
}

// Diff returns an IndexDiff representing changes from idx to next.
func (idx Index) Diff(next Index) (IndexDiff, error) {
	return indexNodeDiff(idx.node, next.node)
}

func indexNodeDiff(a, b pathtree.Node[IndexItem]) (IndexDiff, error) {
	diff := IndexDiff{
		Added:     newEmptyIndex(),
		Removed:   newEmptyIndex(),
		Changed:   newEmptyIndex(),
		Unchanged: newEmptyIndex(),
	}
	// loop over direct children of a (if any)
	for _, aEntr := range a.DirEntries() {
		nA := aEntr.Name() // name of child in a
		chA := a.Child(nA) // child node in a
		chB := b.Child(nA) // child node in b with same name
		if chB == nil {
			// nA is in a, not b -- removed
			diff.addRemoved(nA, chA)
			continue
		}
		// nA exists in both a and b: it may be a directory in both, a file in
		// both, or a directory in one and a file in the other.
		if chA.IsDir() && chB.IsDir() {
			subdiff, err := indexNodeDiff(*chA, *chB)
			if err != nil {
				return IndexDiff{}, err
			}
			diff.mergeDiff(nA, subdiff)
			continue
		}
		if !chA.IsDir() && !chB.IsDir() {
			// chA and chaB are files -- are they the same?
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
		if chA.IsDir() != chB.IsDir() {
			// file in one, directory in the other: chB is new in b and chA
			// doesn't exist in b.
			diff.addAdded(nA, chB)
			diff.addRemoved(nA, chA)
		}
	}
	// loop over direct children of b, looking for added names.
	for _, bEntr := range b.DirEntries() {
		nB := bEntr.Name()
		chB := b.Child(nB)
		if chA := a.Child(nB); chA == nil {
			// nB is in b, not a -- added
			diff.addAdded(nB, chB)
		}
	}
	return diff, nil
}

// IndexDiff represents changes between two indexes.
type IndexDiff struct {
	Added     *Index // exist in second, not first
	Removed   *Index // exist in first, not second
	Changed   *Index // exist in both, changed
	Unchanged *Index // exist in both, unchanged
}

func (diff IndexDiff) Equal() bool {
	if len(diff.Added.node.DirEntries()) > 0 {
		return false
	}
	if len(diff.Removed.node.DirEntries()) > 0 {
		return false
	}
	if len(diff.Changed.node.DirEntries()) > 0 {
		return false
	}
	return true
}

func (diff *IndexDiff) addRemoved(name string, node *pathtree.Node[IndexItem]) {
	if err := diff.Removed.node.Set(name, node); err != nil {
		panic(err)
	}
}

func (diff *IndexDiff) addAdded(name string, node *pathtree.Node[IndexItem]) {
	if err := diff.Added.node.Set(name, node); err != nil {
		panic(err)
	}
}

func (diff *IndexDiff) addChanged(name string, node *pathtree.Node[IndexItem]) {
	if err := diff.Changed.node.Set(name, node); err != nil {
		panic(err)
	}
}

func (diff *IndexDiff) addUnchanged(name string, node *pathtree.Node[IndexItem]) {
	if err := diff.Unchanged.node.Set(name, node); err != nil {
		panic(err)
	}
}

func (diff *IndexDiff) mergeDiff(name string, sub IndexDiff) {
	if len(sub.Added.node.DirEntries()) > 0 {
		diff.addAdded(name, &sub.Added.node)
	}
	if len(sub.Removed.node.DirEntries()) > 0 {
		diff.addRemoved(name, &sub.Removed.node)
	}
	if len(sub.Changed.node.DirEntries()) > 0 {
		diff.addChanged(name, &sub.Changed.node)
	}
	if len(sub.Unchanged.node.DirEntries()) > 0 {
		diff.addUnchanged(name, &sub.Unchanged.node)
	}
}

// IndexItem is a value stored for each path in an Index. In OCFL terms, an
// IndexItems includes information relating to a logical path.
type IndexItem struct {
	// Digests include all digests associated with the path
	Digests digest.Set
	// SrcPaths include all "content paths" (i.e., manifest) entries associated
	// the path
	SrcPaths []string
}

// HasSrc returns true if items includes any names in its SrcPaths
func (item IndexItem) HasSrc(names ...string) bool {
	for _, n := range names {
		for _, p := range item.SrcPaths {
			if p == n {
				return true
			}
		}
	}
	return false
}

// SameContentAs returns true if item and other have in-common digest values or
// SrcPaths. An error is returned if some digests match while others don't --
// this would indicate some inconsistency in the underlying content.
func (item IndexItem) SameContentAs(other IndexItem) (bool, error) {
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
		// some digests matches while others mismatch
		return false, errors.New("inconsistent ")
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
