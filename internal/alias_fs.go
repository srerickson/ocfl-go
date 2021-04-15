package internal

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"sort"
	"time"
)

// AliasFS is used to map logical paths to actual paths
type AliasFS struct {
	base  fs.FS
	index *PathTree
}

func NewAliasFS(fsys fs.FS, files map[string]string) (*AliasFS, error) {
	afs := &AliasFS{fsys, &PathTree{}}
	for from, to := range files {
		err := afs.index.Add(from, to)
		if err != nil {
			return nil, err
		}
	}
	return afs, nil
}

func (afs *AliasFS) Open(name string) (fs.File, error) {
	val, err := afs.index.Get(name)
	if err != nil {
		return nil, err
	}
	switch val := val.(type) {
	case string:
		// path to a regular file
		base, err := afs.base.Open(val)
		if err != nil {
			return nil, err
		}
		return &aliasFile{
			File: base,
			path: name,
		}, nil
	case *PathTree:
		// path to a directory
		dir := &aliasDir{path: name}
		for fname := range val.Files {
			var target string
			if name == "." {
				target = fname
			} else {
				target = name + "/" + fname
			}
			entry := objDirEntry{
				name:  fname,
				isDir: false,
				info:  func() (fs.FileInfo, error) { return afs.Stat(target) },
			}
			dir.entry = append(dir.entry, entry)
		}
		for dname := range val.Dirs {
			var target string
			if name == "." {
				target = dname
			} else {
				target = name + "/" + dname
			}
			entry := objDirEntry{
				name:     dname,
				isDir:    true,
				modeType: fs.ModeDir,
				info:     func() (fs.FileInfo, error) { return afs.Stat(target) },
			}
			dir.entry = append(dir.entry, entry)
		}
		sort.Slice(dir.entry, func(i, j int) bool {
			return dir.entry[i].name < dir.entry[j].name
		})
		return dir, nil
	}
	return nil, errors.New("unexpected value in AliasFS")
}

func (afs *AliasFS) Stat(name string) (fs.FileInfo, error) {
	val, err := afs.index.Get(name)
	if err != nil {
		return nil, err
	}
	switch val := val.(type) {
	case string:
		// val is target file
		info, err := fs.Stat(afs.base, val)
		if err != nil {
			return nil, err
		}
		return &aliasFileInfo{
			info,
			path.Base(name),
		}, nil
	case *PathTree:
		return &dirFileInfo{
			name: path.Base(name),
		}, nil
	}
	return nil, errors.New("unexpected value in AliasFS")
}

type aliasFile struct {
	fs.File
	path string
	info *aliasFileInfo
}

func (file *aliasFile) Stat() (fs.FileInfo, error) {
	if file.info == nil {
		base, err := file.File.Stat()
		if err != nil {
			return nil, err
		}
		file.info = &aliasFileInfo{base, path.Base(file.path)}
	}
	return file.info, nil
}

type aliasFileInfo struct {
	fs.FileInfo
	name string
}

func (info *aliasFileInfo) Name() string {
	return info.name
}

type dirFileInfo struct {
	name string
	// mode    fs.FileMode
	modTime time.Time
	sys     interface{}
}

// implement fs.FileInfo
func (info *dirFileInfo) Name() string       { return info.name }
func (info *dirFileInfo) Size() int64        { return 0 }
func (info *dirFileInfo) Mode() fs.FileMode  { return fs.ModeDir }
func (info *dirFileInfo) ModTime() time.Time { return info.modTime }
func (info *dirFileInfo) IsDir() bool        { return true }
func (info *dirFileInfo) Sys() interface{}   { return info.sys }

// directory open for reading
type aliasDir struct {
	path   string
	entry  []objDirEntry
	offset int
}

func (dir *aliasDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: dir.path, Err: fs.ErrInvalid}
}
func (dir *aliasDir) Close() error { return nil }
func (dir *aliasDir) Stat() (fs.FileInfo, error) {
	return &dirFileInfo{name: path.Base(dir.path)}, nil
}

func (dir *aliasDir) ReadDir(count int) ([]fs.DirEntry, error) {
	n := len(dir.entry) - dir.offset
	if count > 0 && n > count {
		n = count
	}
	if n == 0 && count > 0 {
		return nil, io.EOF
	}
	list := make([]fs.DirEntry, n)
	for i := range list {
		list[i] = &dir.entry[dir.offset+i]
	}
	dir.offset += n
	return list, nil
}

type objDirEntry struct {
	name     string
	isDir    bool
	modeType fs.FileMode
	info     func() (fs.FileInfo, error)
}

func (e *objDirEntry) Name() string               { return e.name }
func (e *objDirEntry) IsDir() bool                { return e.isDir }
func (e *objDirEntry) Type() fs.FileMode          { return e.modeType }
func (e *objDirEntry) Info() (fs.FileInfo, error) { return e.info() }
