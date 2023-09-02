package ocfl

import (
	"errors"
	"fmt"
)

// MapDigestConflictErr indicates same digest found multiple times in the digest map
// (i.e., with different cases)
type MapDigestConflictErr struct {
	Digest string
}

func (d *MapDigestConflictErr) Error() string {
	return fmt.Sprintf("digest conflict for: '%s'", d.Digest)
}

// MapPathConflictErr indicates a path appears more than once in the digest map.
// It's also used in cases where the path as used as a directory in one instance
// and a file in another.
type MapPathConflictErr struct {
	Path string
}

func (p *MapPathConflictErr) Error() string {
	return fmt.Sprintf("path conflict for: '%s'", p.Path)
}

// MapPathInvalidErr indicates an invalid path in a Map.
type MapPathInvalidErr struct {
	Path string
}

func (p *MapPathInvalidErr) Error() string {
	return fmt.Sprintf("invalid path: '%s'", p.Path)
}

// ErrMapMakerExists is returned when calling Add with a path and digest that
// are already present in the MapMaker
var ErrMapMakerExists = errors.New("path and digest exist")
