package ocfl

import (
	"fmt"
	"path/filepath"
	"strings"
)

// EPath represents an OCFL Existing File Path
type EPath string

// LPath represents an OCFL Logial File Path
type LPath string

func NewLPath(path string) (LPath, error) {
	path = filepath.Clean(path)
	if filepath.IsAbs(path) {
		return ``, fmt.Errorf(`Not a relative path: %s`, path)
	}
	if strings.HasPrefix(path, `..`) {
		return ``, fmt.Errorf(`Path out of scope: %s`, path)
	}
	return LPath(filepath.ToSlash(path)), nil
}

func (p LPath) RelPath() string {
	return filepath.FromSlash(string(p))
}

func (p LPath) String() string {
	return string(p)
}
