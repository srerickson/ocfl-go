package version_fs

import (
	"fmt"
	"io/fs"
	"strings"
)

type DirEntries struct {
	Files map[string]string
	Dirs  map[string]*DirEntries
}

type Root DirEntries

func (r *DirEntries) add(fname string, digest string) error {
	if !fs.ValidPath(fname) {
		return fmt.Errorf("invalid path: %s", fname)
	}
	if fname == "." {
		return fmt.Errorf("not a file: %s", fname)
	}
	offset := strings.Index(fname, "/")
	if offset == -1 {
		if r.Files == nil {
			r.Files = make(map[string]string)
		}
		if _, exists := r.Files[fname]; exists {
			return fmt.Errorf("exists: %s", fname)
		}
		if _, exists := r.Dirs[fname]; exists {
			return fmt.Errorf("exists as dir: %s", fname)
		}
		r.Files[fname] = digest
		return nil
	}
	dir := fname[:offset]
	if r.Dirs == nil {
		r.Dirs = make(map[string]*DirEntries)
	}
	if _, exists := r.Files[dir]; exists {
		return fmt.Errorf("exists as file: %s", dir)
	}
	if _, exists := r.Dirs[dir]; !exists {
		r.Dirs[dir] = &DirEntries{}
	}
	return r.Dirs[dir].add(fname[offset+1:], digest)
}
