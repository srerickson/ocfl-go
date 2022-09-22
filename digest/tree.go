package digest

import (
	"io/fs"

	"github.com/srerickson/ocfl/internal/pathtree"
)

var (
	ErrInvalidPath  = pathtree.ErrInvalidPath
	ErrNotFound     = pathtree.ErrNotFound
	ErrNotDir       = pathtree.ErrNotDir
	ErrNotFile      = pathtree.ErrNotFile
	ErrDigestExists = pathtree.ErrValueExists
	// ErrDigestAlg    =
)

// Tree is a data structure for storing digests of files in a directory
// structure, indexed by path.
type Tree struct {
	// root is a paththree that store strings. It corresponds to "."
	root *pathtree.Node[string]
}

type DirEntry interface {
	Name() string
	IsDir() bool
}

// SetDigest sets the digest for a file in the tree. A leaf node and any
// necessary parent nodes (for directories) will be created if they don't exist.
func (t *Tree) SetDigest(p, digest string, replace bool) error {
	if t.root == nil {
		t.root = pathtree.NewDir[string]()
	}
	return pathtree.SetVal(t.root, p, digest, replace)
}

// GetDigest returns the digest for path p.
func (t *Tree) GetDigest(p string) (string, error) {
	if !fs.ValidPath(p) {
		return "", ErrInvalidPath
	}
	if t.root == nil {
		return "", ErrNotFound
	}
	n, err := pathtree.Get(t.root, p)
	if err != nil {
		return "", err
	}
	return n.Val, nil
}

// SetDir attaches the tree sub to t at path p. If replace is false and error is
// returned if path p already exists. If path p exists as a file node and
// replace is true, the file node will be converted to a directory node.
func (t *Tree) SetDir(p string, sub *Tree, replace bool) error {
	if p == "." {
		t.root = sub.root
		return nil
	}
	return pathtree.Set(t.root, p, sub.root, replace)
}

// DirEntries returns ordered slice of DirEntry for contents of p
func (t *Tree) DirEntries(p string) ([]DirEntry, error) {
	if !fs.ValidPath(p) {
		return nil, ErrInvalidPath
	}
	if t.root == nil {
		return nil, nil
	}
	n, err := pathtree.Get(t.root, p)
	if err != nil {
		return nil, err
	}
	children := pathtree.Children(n)
	// don't want to expose pathtree.DirEntry as part of
	// API, so we have to convert/copy slice values.
	// this is ugly but safe
	entries := make([]DirEntry, len(children))
	for i := range children {
		entries[i] = children[i]
	}
	return entries, nil
}

func (t *Tree) Remove(p string, recursive bool) error {
	if !fs.ValidPath(p) {
		return ErrInvalidPath
	}
	if t.root == nil {
		return ErrNotFound
	}
	_, err := pathtree.Remove(t.root, p, recursive)
	pathtree.RemoveEmptyDirs(t.root)
	return err
}

type TreeWalkFunc func(name string, isdir bool, val string) error

func (t *Tree) Walk(fn TreeWalkFunc) error {
	return pathtree.Walk(t.root, (pathtree.WalkFunc[string])(fn))
}

// Let returns number of files in the Tree
func (t *Tree) Len() int {
	if t.root == nil {
		return 0
	}
	var len int
	t.Walk(func(p string, isdir bool, digest string) error {
		if !isdir {
			len++
		}
		return nil
	})
	return len
}

func (t *Tree) AsMap() (*Map, error) {
	m := NewMap()
	if t.root == nil {
		return m, nil
	}
	walkFn := func(p string, isdir bool, digest string) error {
		if isdir {
			return nil
		}
		if err := m.Add(digest, p); err != nil {
			return err
		}
		return nil
	}
	if err := t.Walk(walkFn); err != nil {
		return nil, err
	}
	return m, nil
}

// func (n *node) recursiveDigest(newH func() hash.Hash) (string, error) {
// 	if !n.isDir() {
// 		if n.digest == "" {
// 			return "", fmt.Errorf("missing digest value")
// 		}
// 		return n.digest, nil
// 	}
// 	h := newH()
// 	for _, entr := range n.childrenSorted() {
// 		ch := n.children[entr.Name()]
// 		var isdir uint8
// 		if ch.isDir() {
// 			isdir = 1
// 		}
// 		digest, err := ch.recursiveDigest(newH)
// 		if err != nil {
// 			return "", err
// 		}
// 		_, err = fmt.Fprintf(h, "%x %b %s\n", digest, isdir, entr)
// 		if err != nil {
// 			return "", err
// 		}
// 	}
// 	return hex.EncodeToString(h.Sum(nil)), nil
// }
