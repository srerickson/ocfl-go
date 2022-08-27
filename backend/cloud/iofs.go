package cloud

import (
	"io"
	"io/fs"
	"path"
	"time"
)

type file struct {
	io.ReadCloser
	info *fileInfo
}

func (f file) Stat() (fs.FileInfo, error) {
	return f.info, nil
}

type fileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

// fileInfo implements fs.FileInfo
func (fi fileInfo) Name() string       { return path.Base(fi.name) }
func (fi fileInfo) Size() int64        { return fi.size }
func (fi fileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi fileInfo) ModTime() time.Time { return fi.modTime }
func (fi fileInfo) IsDir() bool        { return fi.mode.IsDir() }
func (fi fileInfo) Sys() interface{}   { return nil }

// fileInfo implements fs.DirEntry
func (fi fileInfo) Type() fs.FileMode          { return fi.Mode().Type() }
func (fi fileInfo) Info() (fs.FileInfo, error) { return fi, nil }

var (
	_ fs.File     = (*file)(nil)
	_ fs.FileInfo = (*fileInfo)(nil)
	_ fs.DirEntry = (*fileInfo)(nil)
)
