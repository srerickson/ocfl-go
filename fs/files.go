package fs

import (
	"context"
	"io/fs"
	"iter"
	"path"
	"strings"
)

func Files(fsys FS, names ...string) iter.Seq[*FileRef] {
	return func(yield func(*FileRef) bool) {
		for _, n := range names {
			ref := &FileRef{
				FS:   fsys,
				Path: n,
			}
			if !yield(ref) {
				break
			}
		}
	}
}

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
func FilterFiles(files iter.Seq[*FileRef], filter func(*FileRef) bool) iter.Seq[*FileRef] {
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

// StatFiles returns an iterator that yields *FileRefs and an error
// from calling [Stat]() for values in files. Values from files are not
// modified.
func StatFiles(ctx context.Context, files iter.Seq[*FileRef]) iter.Seq2[*FileRef, error] {
	newFiles := func(yield func(*FileRef, error) bool) {
		for file := range files {
			newFile := *file
			err := newFile.Stat(ctx)
			if !yield(&newFile, err) {
				break
			}
		}
	}
	return newFiles
}

// IsNotHidden is used with Filter to remove hidden files.
func IsNotHidden(info *FileRef) bool {
	// intentionally ignorng BasePath
	for _, part := range strings.Split(info.Path, "/") {
		if len(part) > 0 && part[0] == '.' {
			return false
		}
	}
	return true
}

// UntilErr returns an iterator that yields the values from seq until seq yields
// a non-nil error. The value yielded with the error is thrown out.
func UntilErr[T any](seq iter.Seq2[T, error]) (iter.Seq[T], func() error) {
	var firstErr error
	outSeq := func(yield func(T) bool) {
		for val, err := range seq {
			if err != nil {
				firstErr = err
				return
			}
			if !yield(val) {
				break
			}
		}
	}
	return outSeq, func() error { return firstErr }
}

// ValidFileType returns true if mode is ok for a file in an OCFL object.
func ValidFileType(mode fs.FileMode) bool {
	// note that the s3 FS implementation uses fs.ModeIrregular
	return mode.IsDir() || mode.IsRegular() || mode.Type() == fs.ModeIrregular
}

// CheckFileTypes checks if items in files have valid mode types for an OCFL
// object (see [ValidFileType]). If will call Stat() on any files with nil
// FileInfo. The resulting iterator yields all files in the input and an error.
// The error is an fs.PathError that wraps ErrFileType if the file has an
// invalid type mode.
func CheckFileTypes(ctx context.Context, files iter.Seq[*FileRef]) iter.Seq2[*FileRef, error] {
	return func(yield func(*FileRef, error) bool) {
		for f := range files {
			var err error
			if f.Info == nil {
				err = f.Stat(ctx)
			}
			if err == nil && !ValidFileType(f.Info.Mode()) {
				err = &fs.PathError{Op: "stat", Path: f.FullPath(), Err: ErrFileType}
			}
			if !yield(f, err) {
				return
			}
		}
	}
}
