package backend

import (
	"io"
	"io/fs"
)

type Writer interface {
	fs.FS
	Write(path string, buffer io.Reader) (int64, error)
}

type Interface interface {
	Writer
	Copy(dst, src string) error
	RemoveAll(path string) error
}

type Renamer interface {
	Interface
	Rename(old, new string) error
}

type StoreScanner interface {
	Interface
	// StoreScan returns a map of paths for OCFL Objects under storage root at
	// path. The map values are Namasted declaration filenames. It returns an
	// error if it encounters an empty directory or a directory with files that
	// are not part of an OCFL Object (excluding the extensions directory).
	StoreScan(path string) (map[string]string, error)
}
