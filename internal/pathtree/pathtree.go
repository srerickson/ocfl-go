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

type Node[T any] struct {
	Val      T                   `json:"v,omitempty"`
	Children map[string]*Node[T] `json:"c,omitempty"`
}

func NewDir[T any]() *Node[T] {
	return &Node[T]{
		Children: make(map[string]*Node[T]),
	}
}

func (node *Node[T]) IsDir() bool {
	return node.Children != nil
}

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

// To prevent cycles the receiver should be the root of the tree.
func (node *Node[T]) Set(p string, child *Node[T], replace bool) error {
	if !fs.ValidPath(p) {
		return ErrInvalidPath
	}
	if p == "." {
		return ErrInvalidPath // cannot set value on self
	}
	if node.IsParent(child) {
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

// SetVal sets the value for a leaf node in the tree. If the path does not
// exist, it is created. If the path refers to a directory node, ErrNotFile is a
// returned. If the path exists and replace is false, ErrValueExists is returned
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
			nextNode = NewDir[T]()
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

func (parent *Node[T]) IsParent(child *Node[T]) bool {
	for _, ch := range parent.Children {
		if ch == child {
			return true
		}
		if ch.IsParent(child) {
			return true
		}
	}
	return false
}

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
