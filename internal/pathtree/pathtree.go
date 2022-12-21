// Package pathree provides Node[T] and generic functions used for storing
// arbitrary values in a hierarchical data structure following filesystem naming
// conventions.
package pathtree

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

var (
	ErrInvalidPath = errors.New("invalid path")
	ErrNotFound    = errors.New("node not found")
	ErrNotDir      = errors.New("not a directory node")
	ErrRelation    = errors.New("cannot add descendant node that is already a descendant or ancestor")
)

// Node is the primary type for pathtree: it includes a value, Val, of generic
// type T and map of named references to direct descendats, Children. Descendant
// nodes are mapped by their name, which cannot include '/'. If Children is nil,
// the node is considered a directory node. Otherwise, it is considered a file
// node.
type Node[T any] struct {
	Val      T
	children map[string]*Node[T]
}

// NewDir returns a new directory node (IsDir() returns true) with no value and
// an empty list of children.
func NewDir[T any]() *Node[T] {
	return &Node[T]{children: make(map[string]*Node[T])}
}

// NewFile returns a new file node with the value val. The returned node
// has no children and IsDir returns false.
func NewFile[T any](val T) *Node[T] {
	return &Node[T]{Val: val}
}

// IsDir indicates whether the node is directory node.
func (node Node[T]) IsDir() bool {
	return node.children != nil
}

// DirEntries returns a sorted DirEntry Sslice representing the children in
// node. If node is not a directory, nil is returned.
func (node Node[T]) DirEntries() []DirEntry {
	if node.children == nil {
		return nil
	}
	entries := make([]DirEntry, len(node.children))
	i := 0
	for n, ch := range node.children {
		entries[i] = DirEntry{
			name:  n,
			isDir: ch.children != nil,
		}
		i++
	}
	sort.Sort(DirEntries(entries))
	return entries
}

// Child returns the node's direct child with the given name. If the the node
// has no child with the given name, or if the node is not a directory node, nil
// is returned.
func (node Node[T]) Child(name string) *Node[T] {
	return node.children[name]
}

// AllPaths returns slice of all path names that are descendants of node
func (node *Node[T]) AllPaths() []string {
	var names []string
	fn := func(name string, node *Node[T]) error {
		if name != "." {
			names = append(names, name)
		}
		return nil
	}
	if err := Walk(node, fn); err != nil {
		panic(err)
	}
	return names
}

// Get returns the node corresponding to the path p, relative to node n. The
// path p must be fs.ValidPath. If p is ".", node is returned.
func (node *Node[T]) Get(p string) (*Node[T], error) {
	if !fs.ValidPath(p) {
		return nil, fmt.Errorf("%w: '%s'", ErrInvalidPath, p)
	}
	if p == "." {
		return node, nil
	}
	for {
		// first/rest...
		first, rest, more := strings.Cut(p, `/`)
		child, exists := node.children[first]
		if !exists {
			return nil, fmt.Errorf("%w: '%s'", ErrNotFound, p)
		}
		node = child
		p = rest
		if !more {
			break
		}
	}
	return node, nil
}

// Set sets child as the Node for path p under node, creating any directory
// nodes if necessary. If p is ".", node's value is set to the same as child.
// An errir is returned if child is a descendant or ancestor of node.
func (node *Node[T]) Set(p string, child *Node[T]) error {
	if !fs.ValidPath(p) {
		return fmt.Errorf("%w: '%s'", ErrInvalidPath, p)
	}
	if p == "." {
		*node = *child
		return nil
	}
	if node.IsParentOf(child) || child.IsParentOf(node) {
		return ErrRelation
	}
	dirName := path.Dir(p)
	baseName := path.Base(p)
	parent, err := node.MkdirAll(dirName)
	if err != nil {
		return fmt.Errorf("making parent directory for %s: %w", p, err)
	}
	parent.children[baseName] = child
	return nil
}

func (node *Node[T]) SetFile(p string, val T) error {
	return node.Set(p, NewFile(val))
}

// MkdirALL creates a directory node named p, along with any necessary parents.
func (node *Node[T]) MkdirAll(p string) (*Node[T], error) {
	if !fs.ValidPath(p) {
		return nil, fmt.Errorf("%w: '%s'", ErrInvalidPath, p)
	}
	if p == "." && node.children != nil {
		return node, nil
	}
	for {
		if node.children == nil {
			return nil, fmt.Errorf("%w: '%s'", ErrNotDir, p)
		}
		first, rest, more := strings.Cut(p, "/")
		nextNode, exists := node.children[first]
		if !exists {
			nextNode = NewDir[T]()
			node.children[first] = nextNode
		}
		node = nextNode
		p = rest
		if !more {
			break
		}
	}
	return node, nil
}

// Remove removes the node for path p and returns it. If p is ".", the node
// value is set to the zero value (of T) and children (if node is a directory)
// are cleared; a copy of the node's former value is returned. An error is
// returned if p is not a ValidPath, or if no node exists for p.
func (node *Node[T]) Remove(p string) (*Node[T], error) {
	if !fs.ValidPath(p) {
		return nil, fmt.Errorf("%w: '%s'", ErrInvalidPath, p)
	}
	if p == "." {
		cp := *node
		if node.IsDir() {
			*node = *NewDir[T]()
		} else {
			*node = Node[T]{}
		}
		return &cp, nil
	}
	dirName := path.Dir(p)
	baseName := path.Base(p)
	parent, err := node.Get(dirName)
	if err != nil {
		return nil, err
	}
	if parent.children == nil {
		return nil, fmt.Errorf("%w: '%s'", ErrNotFound, p)
	}
	ch, exists := parent.children[baseName]
	if !exists {
		return nil, fmt.Errorf("%w: '%s'", ErrNotFound, p)
	}
	delete(parent.children, baseName)
	return ch, nil
}

// Rename moves the node with path from to path to. If replace is true and the
// path to exists, it is overwritten.
func (node *Node[T]) Rename(from, to string) error {
	sub, err := node.Remove(from)
	if err != nil {
		return err
	}
	if err := node.Set(to, sub); err != nil {
		if err := node.Set(from, sub); err != nil {
			// error here is a bug: the node we just removed wasn't re-added
			panic(fmt.Errorf("the removed node couldn't be re-set: %w", err))
		}
		return err
	}
	return nil
}

func (node *Node[T]) Copy() *Node[T] {
	cp := &Node[T]{}
	walkFn := func(name string, n *Node[T]) error {
		var newNode *Node[T]
		if n.IsDir() {
			newNode = NewDir[T]()
			newNode.Val = n.Val
		} else {
			newNode = NewFile(n.Val)
		}
		return cp.Set(name, newNode)
	}
	if err := Walk(node, walkFn); err != nil {
		panic(err)
	}
	return cp
}

// Remove any directories that do not have file nodes as descendants.
func (node *Node[T]) RemoveEmptyDirs() {
	if node.children == nil {
		return
	}
	for name, ch := range node.children {
		ch.RemoveEmptyDirs()
		if ch.children != nil && len(ch.children) == 0 {
			delete(node.children, name)
		}
	}
}

// IsParentOf returns true if child is a descendant of parent.
func (parent *Node[T]) IsParentOf(child *Node[T]) bool {
	for _, ch := range parent.children {
		if ch == child {
			return true
		}
		if ch.IsParentOf(child) {
			return true
		}
	}
	return false
}

// Let returns the number of file nodes under node.
func (node *Node[T]) Len() int {
	var l int
	if node.children == nil {
		return 1
	}
	for _, ch := range node.children {
		l += ch.Len()
	}
	return l
}

// Walk runs fn on every node in the tree
func Walk[T any](node *Node[T], fn WalkFunc[T]) error {
	return walk(node, ".", fn)
}

type WalkFunc[T any] func(name string, node *Node[T]) error

var ErrSkipDir error

func walk[T any](node *Node[T], p string, fn WalkFunc[T]) error {
	if err := fn(p, node); err != nil {
		if err == ErrSkipDir {
			return nil
		}
		return err
	}
	for _, de := range node.DirEntries() {
		child := node.children[de.name]
		chPath := path.Join(p, de.name)
		if err := walk(child, chPath, fn); err != nil {
			return err
		}
	}
	return nil
}

type DirEntry struct {
	name  string
	isDir bool
}

func (de DirEntry) Name() string { return de.name }
func (de DirEntry) IsDir() bool  { return de.isDir }

type DirEntries []DirEntry

func (a DirEntries) Len() int           { return len(a) }
func (a DirEntries) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a DirEntries) Less(i, j int) bool { return a[i].Name() < a[j].Name() }

// Map maps values of type T1 in root to values of type T2 in a new root node.
func Map[T1 any, T2 any](root *Node[T1], fn func(T1) (T2, error)) (*Node[T2], error) {
	newRoot := &Node[T2]{}
	err := Walk(root, func(name string, n *Node[T1]) error {
		newVal, err := fn(n.Val)
		if err != nil {
			return err
		}
		newNode := &Node[T2]{}
		if n.IsDir() {
			newNode.children = make(map[string]*Node[T2])
		}
		newNode.Val = newVal
		return newRoot.Set(name, newNode)
	})
	if err != nil {
		return nil, err
	}
	return newRoot, nil
}
