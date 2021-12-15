package pindex

import (
	"io/fs"
	"strings"
)

var _ PathIndex = (*PathCache)(nil)

type PathCache struct {
	Children map[string]interface{} `json:"C"`
}

func (pc *PathCache) Add(name string, val interface{}) error {
	if !fs.ValidPath(name) {
		return ErrPathInvalid
	}
	if name == "." {
		return ErrPathInvalid
	}
	if pc.Children == nil {
		pc.Children = make(map[string]interface{})
	}
	offset := strings.Index(name, "/")
	if offset == -1 {
		_, exists := pc.Children[name]
		if exists {
			return ErrPathConflict
		}
		pc.Children[name] = val
		return nil
	}
	// name includes at least one "/"
	dir := name[:offset]
	rest := name[offset+1:]
	v, exists := pc.Children[dir]
	if !exists {
		newDir := &PathCache{}
		if err := newDir.Add(rest, val); err != nil {
			return err
		}
		pc.Children[dir] = newDir
		return nil
	}
	switch v := v.(type) {
	case *PathCache:
		return v.Add(rest, val)
	default:
		return ErrPathConflict
	}
}

func (pc *PathCache) Get(name string) (interface{}, error) {
	if !fs.ValidPath(name) {
		return nil, ErrPathInvalid
	}
	if name == "." {
		return pc, nil
	}
	if pc.Children == nil {
		return nil, ErrPathNotFound
	}
	cursor := pc
	var val interface{}
	parts := strings.Split(name, "/")
	for i, n := range parts {
		var exists bool
		val, exists = cursor.Children[n]
		if !exists {
			return nil, ErrPathNotFound
		}
		switch val := val.(type) {
		case *PathCache:
			cursor = val
		default:
			if i < len(parts)-1 {
				return nil, ErrPathNotFound
			}
		}
	}
	return val, nil

}
func (pc *PathCache) Dirs() []string {
	var dirs []string
	for n, v := range pc.Children {
		switch v.(type) {
		case *PathCache:
			dirs = append(dirs, n)
		default:
		}
	}
	return dirs
}
func (pc *PathCache) Files() []string {
	var files []string
	for n, v := range pc.Children {
		switch v.(type) {
		case *PathCache:
		default:
			files = append(files, n)
		}
	}
	return files
}
