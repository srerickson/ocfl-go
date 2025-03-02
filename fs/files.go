package fs

import (
	"context"
	"io/fs"
	"iter"
	"path"
	"strings"
)

type Files iter.Seq[*FileRef]

// FileRef includes everything needed to access a file, including a reference to
// the [FS] where the file is stored. It may include file metadata from calling
// StatFile().
type FileRef struct {
	FS      FS          // The FS where the file is stored.
	BaseDir string      // parent directory relative to an FS.
	Path    string      // file path relative to BaseDir
	Info    fs.FileInfo // file info from StatFile (may be nil)
}

// FullPath returns the file's path relative to an [FS]
func (f FileRef) FullPath() string {
	return path.Join(f.BaseDir, f.Path)
}

// FullPathDir returns the full path of the directory where the
// file is stored.
func (f FileRef) FullPathDir() string {
	return path.Dir(f.FullPath())
}

// Open return an [fs.File] for reading the contents of the file at f.
func (f *FileRef) Open(ctx context.Context) (fs.File, error) {
	return f.FS.OpenFile(ctx, f.FullPath())
}

// Stat() calls StatFile on the file at f and updates f.Info.
func (f *FileRef) Stat(ctx context.Context) error {
	stat, err := StatFile(ctx, f.FS, f.FullPath())
	f.Info = stat
	return err
}

// Filter returns a new FileSeq that yields values in files that satisfy the
// filter condition.
func FilterFiles(files Files, filter func(*FileRef) bool) Files {
	return func(yield func(*FileRef) bool) {
		for ref := range files {
			if !filter(ref) {
				continue
			}
			if !yield(ref) {
				break
			}
		}
	}
}

// IsHidden is used with Filter to remove hidden files.
func IsHidden(info *FileRef) bool {
	// intentionally ignorng BasePath
	for _, part := range strings.Split(info.Path, "/") {
		if len(part) > 0 && part[0] == '.' {
			return false
		}
	}
	return true
}

func WalkFiles(ctx context.Context, fsys FS, dir string) iter.Seq2[*FileRef, error] {
	if walkFS, ok := fsys.(FileWalker); ok {
		return walkFS.WalkFiles(ctx, dir)
	}
	return func(yield func(*FileRef, error) bool) {
		fileWalk(ctx, fsys, dir, ".", yield)
	}
}

// fileWalk calls yield for all files in dir and its subdirectories.
func fileWalk(ctx context.Context, fsys FS, walkRoot string, subDir string, yield func(*FileRef, error) bool) bool {
	for e, err := range ReadDir(ctx, fsys, path.Join(walkRoot, subDir)) {
		if err != nil {
			// any error from ReadDir stops iteration.
			yield(nil, err)
			return false
		}
		entryPath := path.Join(subDir, e.Name())
		switch {
		case e.IsDir():
			if !fileWalk(ctx, fsys, walkRoot, entryPath, yield) {
				return false
			}
		case !validFileType(e.Type()):
			return yield(nil, &fs.PathError{
				Path: entryPath,
				Err:  ErrFileType,
				Op:   `readdir`,
			})
		default:
			info, err := e.Info()
			if err != nil {
				if !yield(nil, err) {
					return false
				}
			}
			ref := &FileRef{
				FS:      fsys,
				BaseDir: walkRoot,
				Path:    entryPath,
				Info:    info,
			}
			if !yield(ref, nil) {
				return false
			}
		}
	}
	return true
}

// validFileType returns true if mode is ok for a file
// in an OCFL object.
func validFileType(mode fs.FileMode) bool {
	return mode.IsDir() || mode.IsRegular() || mode.Type() == fs.ModeIrregular
}
