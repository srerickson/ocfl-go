package alias

import (
	"io/fs"
	"strings"
)

var _ Cache = (*MapCache)(nil)

type MapCache struct {
	Children map[string]interface{} `json:"C"`
}

func (pc *MapCache) Add(name string, val interface{}) error {
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
		newDir := &MapCache{}
		if err := newDir.Add(rest, val); err != nil {
			return err
		}
		pc.Children[dir] = newDir
		return nil
	}
	switch v := v.(type) {
	case *MapCache:
		return v.Add(rest, val)
	default:
		return ErrPathConflict
	}
}

func (pc *MapCache) Get(name string) (interface{}, error) {
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
		case *MapCache:
			cursor = val
		default:
			if i < len(parts)-1 {
				return nil, ErrPathNotFound
			}
		}
	}
	return val, nil

}
func (pc *MapCache) Dirs() []string {
	var dirs []string
	for n, v := range pc.Children {
		switch v.(type) {
		case *MapCache:
			dirs = append(dirs, n)
		default:
		}
	}
	return dirs
}
func (pc *MapCache) Files() []string {
	var files []string
	for n, v := range pc.Children {
		switch v.(type) {
		case *MapCache:
		default:
			files = append(files, n)
		}
	}
	return files
}
