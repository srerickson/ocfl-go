package pindex

import (
	"errors"
	"io/fs"
	"strings"
)

type PathIndex interface {
	Add(string, interface{}) error
	Get(string) (interface{}, error)
	Dirs() []string
	Files() []string
}

var _ PathIndex = (*PathTree)(nil)

// PathTree
type PathTree struct {
	files map[string]interface{}
	dirs  map[string]*PathTree
}

var ErrPathNotFound = errors.New("path not found")
var ErrPathConflict = errors.New("duplicate path")
var ErrPathInvalid = errors.New("invalid path")

func (r *PathTree) Dirs() []string {
	var names []string
	for d := range r.dirs {
		names = append(names, d)
	}
	return names
}

func (r *PathTree) Files() []string {
	var names []string
	for d := range r.files {
		names = append(names, d)
	}
	return names
}

// Add implements PathStore for PathTree
func (r *PathTree) Add(fname string, val interface{}) error {
	if !fs.ValidPath(fname) {
		return ErrPathInvalid
	}
	if fname == "." {
		return ErrPathInvalid
	}
	offset := strings.Index(fname, "/")
	if offset == -1 {
		if _, exists := r.files[fname]; exists {
			return ErrPathConflict
		}
		if _, exists := r.dirs[fname]; exists {
			return ErrPathConflict
		}
		if r.files == nil {
			r.files = make(map[string]interface{})
		}
		r.files[fname] = val
		return nil
	}
	dir := fname[:offset]
	if r.files != nil {
		if _, exists := r.files[dir]; exists {
			return ErrPathConflict
		}
	}
	if r.dirs == nil {
		r.dirs = make(map[string]*PathTree)
	}
	if _, exists := r.dirs[dir]; !exists {
		r.dirs[dir] = &PathTree{}
	}
	return r.dirs[dir].Add(fname[offset+1:], val)
}

// Get implements PathStore for PathTree
func (r *PathTree) Get(fname string) (interface{}, error) {
	if !fs.ValidPath(fname) {
		return "", ErrPathInvalid
	}
	if fname == "." {
		return r, nil
	}
	offset := strings.Index(fname, "/")
	if offset == -1 {
		if r.files != nil {
			val, exists := r.files[fname]
			if exists {
				return val, nil
			}
		}
		if r.dirs != nil {
			val, exists := r.dirs[fname]
			if exists {
				return val, nil
			}
		}
		return "", ErrPathNotFound
	}
	dir := fname[:offset]
	if r.dirs == nil {
		r.dirs = make(map[string]*PathTree)
	}
	val, exists := r.dirs[dir]
	if !exists {
		return "", ErrPathNotFound
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
