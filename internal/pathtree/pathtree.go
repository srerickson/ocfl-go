package pathtree

import (
	"errors"
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
)

type Node[T any] struct {
	Val      T
	children map[string]*Node[T]
}

func NewDir[T any]() *Node[T] {
	return &Node[T]{
		children: make(map[string]*Node[T]),
	}
}

func IsDir[T any](node *Node[T]) bool {
	return node.children != nil
}

func Children[T any](node *Node[T]) []DirEntry {
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
	sort.Sort(dirEntries(entries))
	return entries
}

func Get[T any](node *Node[T], p string) (*Node[T], error) {
	if p == "." {
		return node, nil
	}
	for {
		first, rest, more := strings.Cut(p, `/`)
		child, exists := node.children[first]
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

func Set[T any](node *Node[T], p string, child *Node[T], replace bool) error {
	if !fs.ValidPath(p) {
		return ErrInvalidPath
	}
	if p == "." {
		return ErrInvalidPath // cannot set value on self
	}
	dirName := path.Dir(p)
	baseName := path.Base(p)
	parent, err := MkdirAll(node, dirName)
	if err != nil {
		return err
	}
	_, exists := parent.children[baseName]
	if exists && !replace {
		return ErrValueExists
	}
	parent.children[baseName] = child
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
	parent, err := MkdirAll(node, dirName)
	if err != nil {
		return err
	}
	child, exists := parent.children[baseName]
	if !exists {
		parent.children[baseName] = &Node[T]{Val: val}
		return nil
	}
	if child.children != nil {
		return ErrNotFile
	}
	if !replace {
		return ErrValueExists
	}
	child.Val = val
	return nil
}

func MkdirAll[T any](node *Node[T], p string) (*Node[T], error) {
	if !fs.ValidPath(p) {
		return nil, ErrInvalidPath
	}
	if p == "." && node.children != nil {
		return node, nil
	}
	for {
		if node.children == nil {
			return nil, ErrNotDir
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

func Remove[T any](node *Node[T], p string, recursive bool) (*Node[T], error) {
	if !fs.ValidPath(p) {
		return nil, ErrInvalidPath
	}
	if p == "." {
		return nil, ErrInvalidPath
	}
	dirName := path.Dir(p)
	baseName := path.Base(p)
	parent, err := Get(node, dirName)
	if err != nil {
		return nil, err
	}
	if parent.children == nil {
		return nil, ErrNotFound
	}
	ch, exists := parent.children[baseName]
	if !exists {
		return nil, ErrNotFound
	}
	if IsDir(ch) && !recursive {
		return nil, ErrNotFile
	}
	delete(parent.children, baseName)
	return ch, nil
}

// Remove any directories that do not have file nodes as descendants.
func RemoveEmptyDirs[T any](node *Node[T]) {
	if node.children == nil {
		return
	}
	for name, ch := range node.children {
		RemoveEmptyDirs(ch)
		if ch.children != nil && len(ch.children) == 0 {
			delete(node.children, name)
		}
	}
}

// Walk runs fn on every node in the tree
func Walk[T any](node *Node[T], fn WalkFunc[T]) error {
	return walk(node, ".", fn)
}

type WalkFunc[T any] func(name string, isdir bool, val T) error

var ErrSkipDir error

func walk[T any](node *Node[T], p string, fn WalkFunc[T]) error {
	isdir := node.children != nil
	if err := fn(p, isdir, node.Val); err != nil {
		if err == ErrSkipDir {
			return nil
		}
		return err
	}
	for _, de := range Children(node) {
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

type dirEntries []DirEntry

func (a dirEntries) Len() int           { return len(a) }
func (a dirEntries) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a dirEntries) Less(i, j int) bool { return a[i].Name() < a[j].Name() }
