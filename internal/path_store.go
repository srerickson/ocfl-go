package internal

import (
	"fmt"
	"io/fs"
	"strings"
)

type PathStore interface {
	Add(string, interface{}) error
	Get(string) (interface{}, error)
}

// PathTree
type PathTree struct {
	Files map[string]interface{}
	Dirs  map[string]*PathTree
}

// Add implements PathStore for PathTree
func (r *PathTree) Add(fname string, val interface{}) error {
	if !fs.ValidPath(fname) {
		return fmt.Errorf("invalid path: %s", fname)
	}
	if fname == "." {
		return fmt.Errorf("not a file: %s", fname)
	}
	offset := strings.Index(fname, "/")
	if offset == -1 {
		if _, exists := r.Files[fname]; exists {
			return fmt.Errorf("exists: %s", fname)
		}
		if _, exists := r.Dirs[fname]; exists {
			return fmt.Errorf("exists as dir: %s", fname)
		}
		if r.Files == nil {
			r.Files = make(map[string]interface{})
		}
		r.Files[fname] = val
		return nil
	}
	dir := fname[:offset]
	if r.Files != nil {
		if _, exists := r.Files[dir]; exists {
			return fmt.Errorf("exists as file: %s", dir)
		}
	}
	if r.Dirs == nil {
		r.Dirs = make(map[string]*PathTree)
	}
	if _, exists := r.Dirs[dir]; !exists {
		r.Dirs[dir] = &PathTree{}
	}
	return r.Dirs[dir].Add(fname[offset+1:], val)
}

// Get implements PathStore for PathTree
func (r *PathTree) Get(fname string) (interface{}, error) {
	if !fs.ValidPath(fname) {
		return "", fmt.Errorf("invalid path: %s", fname)
	}
	if fname == "." {
		return r, nil
	}
	offset := strings.Index(fname, "/")
	if offset == -1 {
		if r.Files != nil {
			val, exists := r.Files[fname]
			if exists {
				return val, nil
			}
		}
		if r.Dirs != nil {
			val, exists := r.Dirs[fname]
			if exists {
				return val, nil
			}
		}
		return "", fmt.Errorf(`not found`)
	}
	dir := fname[:offset]
	if r.Dirs == nil {
		r.Dirs = make(map[string]*PathTree)
	}
	val, exists := r.Dirs[dir]
	if !exists {
		return "", fmt.Errorf(`not found`)
	}
	return val.Get(fname[offset+1:])
}

// //ImpB
// type ImpB map[string]*Entry

// type Entry struct {
// 	val   interface{}
// 	isDir bool
// }

// func (r ImpB) Add(fname string, val interface{}) error {
// 	return nil
// }

// func (fsys ImpB) Get(name string) (interface{}, error) {
// 	if !fs.ValidPath(name) {
// 		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
// 	}
// 	file := fsys[name]
// 	if file != nil && !file.isDir {
// 		return file.val, nil
// 	}
// 	var list []Entry
// 	var elem string
// 	var need = make(map[string]bool)
// 	if name == "." {
// 		elem = "."
// 		for fname, f := range fsys {
// 			i := strings.Index(fname, "/")
// 			if i < 0 {
// 				list = append(list, mapFileInfo{fname, f})
// 			} else {
// 				need[fname[:i]] = true
// 			}
// 		}
// 	} else {
// 		elem = name[strings.LastIndex(name, "/")+1:]
// 		prefix := name + "/"
// 		for fname, f := range fsys {
// 			if strings.HasPrefix(fname, prefix) {
// 				felem := fname[len(prefix):]
// 				i := strings.Index(felem, "/")
// 				if i < 0 {
// 					list = append(list, mapFileInfo{felem, f})
// 				} else {
// 					need[fname[len(prefix):len(prefix)+i]] = true
// 				}
// 			}
// 		}
// 		// If the directory name is not in the map,
// 		// and there are no children of the name in the map,
// 		// then the directory is treated as not existing.
// 		if file == nil && list == nil && len(need) == 0 {
// 			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
// 		}
// 	}
// 	for _, fi := range list {
// 		delete(need, fi.name)
// 	}
// 	for name := range need {
// 		list = append(list, mapFileInfo{name, &MapFile{Mode: fs.ModeDir}})
// 	}
// 	sort.Slice(list, func(i, j int) bool {
// 		return list[i].name < list[j].name
// 	})
// 	if file == nil {
// 		file = &MapFile{Mode: fs.ModeDir}
// 	}
// 	return &mapDir{name, mapFileInfo{elem, file}, list, 0}, nil
// }
