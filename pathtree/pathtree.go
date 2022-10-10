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
	ErrNotFile     = errors.New("not a file node")
	ErrValueExists = errors.New("cannot replace existing node value")
	ErrCycle       = errors.New("this operation may introduce a cycle")
)

// Node is the primary type for pathtree: it includes a value, Val, of generic
// type T and map of named references to direct descendats, Children. Descendant
// nodes are mapped by their name, which cannot include '/'. If Children is nil,
// the node is considered a directory node. Otherwise, it is considered a file
// node.
type Node[T any] struct {
	Val      T                   `json:"v,omitempty"`
	Children map[string]*Node[T] `json:"c,omitempty"`
}

// NewRoot returns a new directory node for storing values of type T in
// descendant nodes.
func NewRoot[T any]() *Node[T] {
	return &Node[T]{
		Children: make(map[string]*Node[T]),
	}
}

// IsDir indicates whther the node is directory node.
func (node *Node[T]) IsDir() bool {
	return node.Children != nil
}

// ReadDir returns a sorted slice of DirEntry structs representing the children
// in node. If node is not a directory, nil is returned.
func (node *Node[T]) ReadDir() []DirEntry {
	if node.Children == nil {
		return nil
	}
	entries := make([]DirEntry, len(node.Children))
	i := 0
	for n, ch := range node.Children {
		entries[i] = DirEntry{
			name:  n,
			isDir: ch.Children != nil,
		}
		i++
	}
	sort.Sort(DirEntries(entries))
	return entries
}

// Get returns the node corresponding to the path p, relative to node n.
// The path p must be fs.ValidPath
func (node *Node[T]) Get(p string) (*Node[T], error) {
	if !fs.ValidPath(p) {
		return nil, ErrInvalidPath
	}
	if p == "." {
		return node, nil
	}
	for {
		first, rest, more := strings.Cut(p, `/`)
		child, exists := node.Children[first]
		if !exists {
			return nil, ErrNotFound
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
// nodes if necessary. For example, for a path 'a/b/c.txt', Set will create
// directory nodes for a and a/b if they do not already exist. If a node for p
// exists and and replace is true, the existing node is replaced with child;
// otherwise, an error is returned.
func (node *Node[T]) Set(p string, child *Node[T], replace bool) error {
	if !fs.ValidPath(p) {
		return ErrInvalidPath
	}
	if p == "." {
		if !replace {
			return ErrValueExists
		}
		node.Children = child.Children
		node.Val = child.Val
		return nil
	}
	if node.IsParentOf(child) {
		return fmt.Errorf("node is alread part of the tree: %w", ErrCycle)
	}
	dirName := path.Dir(p)
	baseName := path.Base(p)
	parent, err := node.MkdirAll(dirName)
	if err != nil {
		return err
	}
	_, exists := parent.Children[baseName]
	if exists && !replace {
		return ErrValueExists
	}
	parent.Children[baseName] = child
	return nil
}

// SetVal sets the value of the child node for path p under node. If the path
// does not exist, it is created. If the path refers to a directory node,
// ErrNotFile is a returned. If the path exists and replace is false,
// ErrValueExists is returned
func SetVal[T any](node *Node[T], p string, val T, replace bool) error {
	if !fs.ValidPath(p) {
		return ErrInvalidPath
	}
	if p == "." {
		return ErrNotFile
	}
	dirName := path.Dir(p)
	baseName := path.Base(p)
	parent, err := node.MkdirAll(dirName)
	if err != nil {
		return err
	}
	child, exists := parent.Children[baseName]
	if !exists {
		parent.Children[baseName] = &Node[T]{Val: val}
		return nil
	}
	if child.Children != nil {
		return ErrNotFile
	}
	if !replace {
		return ErrValueExists
	}
	child.Val = val
	return nil
}

// MkdirALL creates a directory node named p, along with any necessary parents.
func (node *Node[T]) MkdirAll(p string) (*Node[T], error) {
	if !fs.ValidPath(p) {
		return nil, ErrInvalidPath
	}
	if p == "." && node.Children != nil {
		return node, nil
	}
	for {
		if node.Children == nil {
			return nil, ErrNotDir
		}
		first, rest, more := strings.Cut(p, "/")
		nextNode, exists := node.Children[first]
		if !exists {
			nextNode = NewRoot[T]()
			node.Children[first] = nextNode
		}
		node = nextNode
		p = rest
		if !more {
			break
		}
	}
	return node, nil
}

// Remove removes the node for path p and returns it. If the node is a directory
// node, recursive must be true or an error is returned.
func (node *Node[T]) Remove(p string, recursive bool) (*Node[T], error) {
	if !fs.ValidPath(p) {
		return nil, ErrInvalidPath
	}
	if p == "." {
		return nil, ErrInvalidPath
	}
	dirName := path.Dir(p)
	baseName := path.Base(p)
	parent, err := node.Get(dirName)
	if err != nil {
		return nil, err
	}
	if parent.Children == nil {
		return nil, ErrNotFound
	}
	ch, exists := parent.Children[baseName]
	if !exists {
		return nil, ErrNotFound
	}
	if ch.IsDir() && !recursive {
		return nil, ErrNotFile
	}
	delete(parent.Children, baseName)
	return ch, nil
}

// Remove any directories that do not have file nodes as descendants.
func (node *Node[T]) RemoveEmptyDirs() {
	if node.Children == nil {
		return
	}
	for name, ch := range node.Children {
		ch.RemoveEmptyDirs()
		if ch.Children != nil && len(ch.Children) == 0 {
			delete(node.Children, name)
		}
	}
}

// IsParentOf returns true if child is a descendant of parent.
func (parent *Node[T]) IsParentOf(child *Node[T]) bool {
	for _, ch := range parent.Children {
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
	if node.Children == nil {
		return 1
	}
	for _, ch := range node.Children {
		l += ch.Len()
	}
	return l
}

// check that all nodes are referenced only once
func (root *Node[T]) HasCycle() bool {
	refs := map[*Node[T]]struct{}{}
	return hasCycleVisit(refs, root)
}

func hasCycleVisit[T any](refs map[*Node[T]]struct{}, n *Node[T]) bool {
	if _, exists := refs[n]; exists {
		return true
	}
	refs[n] = struct{}{}
	for _, ch := range n.Children {
		if hasCycleVisit(refs, ch) {
			return true
		}
	}
	return false
}

// Walk runs fn on every node in the tree
func Walk[T any](node *Node[T], fn WalkFunc[T]) error {
	return walk(node, ".", fn)
}

type WalkFunc[T any] func(name string, isdir bool, val T) error

var ErrSkipDir error

func walk[T any](node *Node[T], p string, fn WalkFunc[T]) error {
	isdir := node.Children != nil
	if err := fn(p, isdir, node.Val); err != nil {
		if err == ErrSkipDir {
			return nil
		}
		return err
	}
	for _, de := range node.ReadDir() {
		child := node.Children[de.name]
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
	err := Walk(root, func(name string, isdir bool, val T1) error {
		newVal, err := fn(val)
		if err != nil {
			return err
		}
		newNode := &Node[T2]{}
		if isdir {
			newNode.Children = make(map[string]*Node[T2])
		}
		newNode.Val = newVal
		return newRoot.Set(name, newNode, true)
	})
	if err != nil {
		return nil, err
	}
	return newRoot, nil
}
