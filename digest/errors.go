package digest

import "fmt"

// DigestErr is returned when content's digest conflicts with an expected value
type DigestErr struct {
	Name     string // Content path
	AlgID    string // Digest algorithm
	Got      string // Calculated digest
	Expected string // Expected digest
}

func (e DigestErr) Error() string {
	if e.Name == "" {
		return fmt.Sprintf("unexpected %s: %s, got: %s", e.AlgID, e.Got, e.Expected)
	}
	return fmt.Sprintf("unexpected %s for '%s': %s, got: %s", e.AlgID, e.Name, e.Got, e.Expected)
}

// MapDigestConflictErr indicates same digest found multiple times in the digest map
// (i.e., with different cases)
type MapDigestConflictErr struct {
	Digest string
}

func (d *MapDigestConflictErr) Error() string {
	return "digest conflict: " + string(d.Digest)
}

// MapPathConflictErr indicates a path appears more than once in the digest map.
// It's also used in cases where the path as used as a directory in one instance
// and a file in another.
type MapPathConflictErr struct {
	Path string
}

func (p *MapPathConflictErr) Error() string {
	return "path conflict: " + string(p.Path)
}

// MapPathInvalidErr indicates an invalid path in a Map.
type MapPathInvalidErr struct {
	Path string
}

func (p *MapPathInvalidErr) Error() string {
	return "invalid path: " + string(p.Path)
}
