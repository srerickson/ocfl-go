package logical

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	ocflfs "github.com/srerickson/ocfl-go/fs"
)

// NewLogicalFS returns a new LogicalFS that uses refs to map logical file names
// to names in fsys. The LogicalFS's directory structure is infered from the
// file names (keys) in refs. The given context will be used for read operations
// to the underlying ocfl.FS. Files modtime is set with created.
func NewLogicalFS(ctx context.Context, fsys ocflfs.FS, refs map[string]string, created time.Time) *LogicalFS {
	return &LogicalFS{
		ctx:     ctx,
		fs:      fsys,
		refs:    refs,
		created: created,
	}
}

// LogicalFS implements io/fs.FS using logical mapping of files to an underlying
// ocfl.FS.
type LogicalFS struct {
	ctx context.Context
	// underlying storage
	fs ocflfs.FS
	// map logical file names to real filenames
	refs    map[string]string
	created time.Time
}

var _ fs.FS = (*LogicalFS)(nil)

func (fsys *LogicalFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Err:  fs.ErrInvalid,
			Op:   "open",
			Path: name,
		}
	}
	if name == "." {
		return fsys.openDir(".")
	}
	return fsys.openFile(name)
}

func (fsys *LogicalFS) openFile(name string) (fs.File, error) {
	realName := fsys.refs[name]
	if realName == "" {
		// name doesn't exist in state.
		// try opening as a directory
		return fsys.openDir(name)
	}
	f, err := fsys.fs.OpenFile(fsys.ctx, realName)
	if err != nil {
		err = fmt.Errorf("opening file with logical path %q: %w", name, err)
		return nil, err
	}
	logical := &logicalFile{
		File:    f,
		mode:    0444,
		created: fsys.created,
		name:    path.Base(name),
	}
	return logical, nil
}

func (fsys *LogicalFS) openDir(dir string) (fs.ReadDirFile, error) {
	prefix := dir + "/"
	if prefix == "./" {
		prefix = ""
	}
	children := map[string]*logicalDirEntry{}
	for p := range fsys.refs {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		childName, _, childIsDir := strings.Cut(strings.TrimPrefix(p, prefix), "/")
		if _, exists := children[childName]; exists {
			continue
		}
		entry := &logicalDirEntry{
			name:    childName,
			mode:    0444,
			created: fsys.created,
			open:    func() (fs.File, error) { return fsys.Open(path.Join(dir, childName)) },
		}
		if childIsDir {
			entry.mode = fs.ModeDir | 0555
		}
		children[childName] = entry
	}
	if dir != "." && len(children) < 1 {
		return nil, &fs.PathError{
			Op:   "open",
			Path: dir,
			Err:  fs.ErrNotExist,
		}
	}
	dirFile := &logicalFile{
		name:    path.Base(dir),
		entries: make([]fs.DirEntry, 0, len(children)),
		mode:    fs.ModeDir | 0555,
		created: fsys.created,
	}
	for _, entry := range children {
		dirFile.entries = append(dirFile.entries, entry)
	}
	slices.SortFunc(dirFile.entries, func(a, b fs.DirEntry) int {
		return cmp.Compare(a.Name(), b.Name())
	})
	return dirFile, nil
}

type logicalFile struct {
	fs.File
	name    string
	created time.Time
	mode    fs.FileMode
	size    int64
	// for  directories
	entries []fs.DirEntry
	offset  int
}

var _ fs.ReadDirFile = (*logicalFile)(nil)
var _ fs.File = (*logicalFile)(nil)
var _ fs.FileInfo = (*logicalFile)(nil)

func (dir *logicalFile) Close() error {
	if dir.File != nil {
		return dir.File.Close()
	}
	return nil
}
func (f *logicalFile) IsDir() bool        { return f.mode.IsDir() }
func (f *logicalFile) Mode() fs.FileMode  { return f.mode }
func (f *logicalFile) ModTime() time.Time { return f.created }
func (f *logicalFile) Name() string       { return f.name }
func (f *logicalFile) Read(b []byte) (int, error) {
	if f.File == nil {
		return 0, &fs.PathError{Op: "read", Err: errors.New("is a directory")}
	}
	return f.File.Read(b)
}

func (f *logicalFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		entries := f.entries[f.offset:]
		f.offset = len(f.entries)
		return entries, nil
	}
	if remain := len(f.entries) - f.offset; remain < n {
		n = remain
	}
	if n <= 0 {
		return nil, io.EOF
	}
	entries := f.entries[f.offset : f.offset+n]
	f.offset += n
	return entries, nil
}
func (f *logicalFile) Size() int64 { return f.size }
func (f *logicalFile) Stat() (fs.FileInfo, error) {
	if f.File != nil {
		info, err := f.File.Stat()
		if err != nil {
			return nil, err
		}
		f.size = info.Size()
	}
	return f, nil
}
func (f *logicalFile) Sys() any { return nil }

type logicalDirEntry struct {
	name    string
	created time.Time
	mode    fs.FileMode
	open    func() (fs.File, error)
}

var _ fs.DirEntry = (*logicalDirEntry)(nil)

func (e *logicalDirEntry) Name() string      { return e.name }
func (e *logicalDirEntry) IsDir() bool       { return e.mode.IsDir() }
func (e *logicalDirEntry) Type() fs.FileMode { return e.mode.Type() }

func (e *logicalDirEntry) Info() (fs.FileInfo, error) {
	f, err := e.open()
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	return stat, errors.Join(err, f.Close())
}

func (e *logicalDirEntry) Mode() fs.FileMode  { return e.mode }
func (e *logicalDirEntry) ModTime() time.Time { return e.created }
func (e *logicalDirEntry) Sys() any           { return nil }
