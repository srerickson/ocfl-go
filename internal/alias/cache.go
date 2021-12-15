package alias

import (
	"errors"
)

// Cache stores info related to logical paths, used by alias.FS
type Cache interface {
	Add(string, interface{}) error
	Get(string) (interface{}, error)
	Dirs() []string
	Files() []string
}

var ErrPathNotFound = errors.New("path not found")
var ErrPathConflict = errors.New("duplicate path")
var ErrPathInvalid = errors.New("invalid path")
