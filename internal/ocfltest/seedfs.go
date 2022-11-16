// SeedFS is based on `testing/fstest/mapfs.go` from the go standard libary,
// which is distributed with the following license:
//
//
// Copyright (c) 2009 The Go Authors. All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//    * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//    * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//    * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package ocfltest

import (
	"io"
	"io/fs"
	"math/rand"
	"path"
	"sort"
	"strings"
	"time"
)

type SeedFS map[string]*SeedFile

type SeedFile struct {
	Seed    int64 // seed used by rand.NewSource for file content
	Size    int64 // size of file
	Mode    fs.FileMode
	Sys     any
	ModTime time.Time
}

func (fsys SeedFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	file := fsys[name]
	if file != nil && !file.Mode.IsDir() {
		return &openSeedFile{
			Reader: rand.New(rand.NewSource(file.Seed)),
			info:   &seedFileInfo{name: path.Base(name), file: file},
		}, nil
	}

	// Directory, possibly synthesized.
	// Note that file can be nil here: the map need not contain explicit parent directories for all its files.
	// But file can also be non-nil, in case the user wants to set metadata for the directory explicitly.
	// Either way, we need to construct the list of children of this directory.
	var list []seedFileInfo
	var elem string
	var need = make(map[string]bool)
	if name == "." {
		elem = "."
		for fname, f := range fsys {
			i := strings.Index(fname, "/")
			if i < 0 {
				if fname != "." {
					list = append(list, seedFileInfo{fname, f})
				}
			} else {
				need[fname[:i]] = true
			}
		}
	} else {
		elem = name[strings.LastIndex(name, "/")+1:]
		prefix := name + "/"
		for fname, f := range fsys {
			if strings.HasPrefix(fname, prefix) {
				felem := fname[len(prefix):]
				i := strings.Index(felem, "/")
				if i < 0 {
					list = append(list, seedFileInfo{felem, f})
				} else {
					need[fname[len(prefix):len(prefix)+i]] = true
				}
			}
		}
		// If the directory name is not in the map,
		// and there are no children of the name in the map,
		// then the directory is treated as not existing.
		if file == nil && list == nil && len(need) == 0 {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
	}
	for _, fi := range list {
		delete(need, fi.name)
	}
	for name := range need {
		list = append(list, seedFileInfo{name, &SeedFile{Mode: fs.ModeDir}})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].name < list[j].name
	})

	if file == nil {
		file = &SeedFile{Mode: fs.ModeDir}
	}
	return &seedDir{name, seedFileInfo{elem, file}, list, 0}, nil
}

type openSeedFile struct {
	io.Reader
	offset int
	info   *seedFileInfo
}

func (file *openSeedFile) Read(b []byte) (int, error) {
	total := int(file.info.Size())
	if file.offset >= total {
		return 0, io.EOF
	}
	if r := total - file.offset; r < len(b) {
		n, err := file.Reader.Read(b[:r])
		file.offset += n
		return n, err

	}
	n, err := file.Reader.Read(b)
	file.offset += n
	return n, err
}

func (file openSeedFile) Stat() (fs.FileInfo, error) {
	return file.info, nil
}

type seedFileInfo struct {
	name string
	file *SeedFile
}

func (info seedFileInfo) Name() string               { return info.name }
func (info seedFileInfo) Size() int64                { return info.file.Size }
func (info seedFileInfo) Mode() fs.FileMode          { return info.file.Mode }
func (info seedFileInfo) Type() fs.FileMode          { return info.file.Mode.Type() }
func (info seedFileInfo) ModTime() time.Time         { return info.file.ModTime }
func (info seedFileInfo) IsDir() bool                { return info.file.Mode.IsDir() }
func (info seedFileInfo) Sys() any                   { return info.file.Sys }
func (info seedFileInfo) Info() (fs.FileInfo, error) { return info, nil }

func (file *openSeedFile) Close() error {
	return nil
}

type seedDir struct {
	path string
	seedFileInfo
	entry  []seedFileInfo
	offset int
}

func (d *seedDir) Stat() (fs.FileInfo, error) { return &d.seedFileInfo, nil }
func (d *seedDir) Close() error               { return nil }
func (d *seedDir) Read(b []byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.path, Err: fs.ErrInvalid}
}

func (d *seedDir) ReadDir(count int) ([]fs.DirEntry, error) {
	n := len(d.entry) - d.offset
	if n == 0 && count > 0 {
		return nil, io.EOF
	}
	if count > 0 && n > count {
		n = count
	}
	list := make([]fs.DirEntry, n)
	for i := range list {
		list[i] = &d.entry[d.offset+i]
	}
	d.offset += n
	return list, nil
}
