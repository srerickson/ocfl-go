package ocfl

import (
	"io"
	"io/fs"
	"path"
	"sort"
	"time"

	"github.com/srerickson/ocfl/internal"
)

// regular file open for reading
type objFile struct {
	path string
	info fs.FileInfo
	file fs.File // actual file open for reading
}

func (f *objFile) Stat() (fs.FileInfo, error) {
	if f.info != nil {
		return f.info, nil
	}
	realInfo, err := f.file.Stat()
	if err != nil {
		return nil, err
	}
	f.info = renameFileInfo(realInfo, path.Base(f.path))
	return f.info, nil
}
func (f *objFile) Read(b []byte) (int, error) { return f.file.Read(b) }
func (f *objFile) Close() error               { return f.file.Close() }

type objFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	sys     interface{}
}

// implement fs.FileInfo
func (info *objFileInfo) Name() string       { return info.name }
func (info *objFileInfo) Size() int64        { return info.size }
func (info *objFileInfo) Mode() fs.FileMode  { return info.mode }
func (info *objFileInfo) ModTime() time.Time { return info.modTime }
func (info *objFileInfo) IsDir() bool        { return info.Mode().IsDir() }
func (info *objFileInfo) Sys() interface{}   { return info.sys }

// directory open for reading
type objDir struct {
	path   string
	entry  []objDirEntry
	offset int
}

func (dir *objDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: dir.path, Err: fs.ErrInvalid}
}

func (dir *objDir) Close() error { return nil }
func (dir *objDir) Stat() (fs.FileInfo, error) {
	return &objFileInfo{name: path.Base(dir.path), mode: fs.ModeDir}, nil
}

func (dir *objDir) ReadDir(count int) ([]fs.DirEntry, error) {
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

// Open implements io/fs.FS for ObjectReader
func (obj *ObjectReader) Open(name string) (fs.File, error) {
	val, err := obj.index.Get(name)
	if err != nil {
		return nil, err
	}
	switch val := val.(type) {
	case string:
		realF, err := obj.openFile(val)
		if err != nil {
			return nil, err
		}
		return &objFile{path: name, file: realF}, nil
	case *internal.PathTree:
		dirEntry := &objDir{path: name}
		for fname, d := range val.Files {
			entry := objDirEntry{
				name:  fname,
				isDir: false,
				info:  obj.statFunc(fname, d),
			}
			dirEntry.entry = append(dirEntry.entry, entry)
		}
		for dname, treeNode := range val.Dirs {
			entry := objDirEntry{
				name:     dname,
				isDir:    true,
				modeType: fs.ModeDir,
				info:     obj.statFunc(dname, treeNode),
			}
			dirEntry.entry = append(dirEntry.entry, entry)
		}
		sort.Slice(dirEntry.entry, func(i, j int) bool {
			return dirEntry.entry[i].name < dirEntry.entry[j].name
		})

		return dirEntry, nil
	}
	return nil, nil
}

func (obj *ObjectReader) openFile(digest string) (fs.File, error) {
	paths, ok := obj.inventory.Manifest[digest]
	if !ok || len(paths) == 0 {
		return nil, fs.ErrNotExist
	}
	return obj.root.Open(paths[0])
}

// returns a function that returns fileinfo for the path
func (obj *ObjectReader) statFunc(name string, node interface{}) func() (fs.FileInfo, error) {
	return func() (fs.FileInfo, error) {
		var info fs.FileInfo
		switch val := node.(type) {
		case string:
			realF, err := obj.openFile(val)
			if err != nil {
				return nil, err
			}
			defer realF.Close()
			realInfo, err := realF.Stat()
			if err != nil {
				return nil, err
			}
			info = renameFileInfo(realInfo, name)
		case *internal.PathTree:
			info = &objFileInfo{
				name: name,
				size: 0,
				mode: fs.ModeDir,
				sys:  nil,
			}
		}
		return info, nil
	}
}

func renameFileInfo(in fs.FileInfo, newname string) fs.FileInfo {
	return &objFileInfo{
		name:    newname,
		size:    in.Size(),
		mode:    in.Mode(),
		modTime: in.ModTime(),
		sys:     nil,
	}
}
