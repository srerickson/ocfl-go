package ocfl

import (
	"strings"

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

func (idx Index) String() string { return strings.Join(idx.node.AllPaths(), ", ") }

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

// SetRoot is used internally to set the contents of an Index. It shouldn't be
// usable outside this package or its subpackages.
func (idx *Index) SetRoot(root *pathtree.Node[IndexItem]) {
	idx.node = *root
}

// return an empty directory index
func newEmptyIndex() *Index {
	return &Index{node: *pathtree.NewDir[IndexItem]()}
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
