package digest

import (
	"fmt"
	"io"
	"strings"
)

// Digester is an interface used for generating values.
type Digester interface {
	io.Writer
	// String() returns the value for the bytes written to the r.
	String() string
}

// NewDigester is a convenience function that returns a new
// for a built-in Alg with the given id.
func NewDigester(id string) (Digester, error) {
	return builtinRegister.NewDigester(id)
}

// MultiDigester is used to generate  for multiple algorithms at
// the same time.
type MultiDigester struct {
	io.Writer
	digesters map[string]Digester
}

// NewMultiDigester constructs a MultiDigester for one or more built-in digest
// algorithms.
func NewMultiDigester(algs ...string) *MultiDigester {
	writers := make([]io.Writer, 0, len(algs))
	rs := make(map[string]Digester, len(algs))
	for _, algID := range algs {
		r, _ := NewDigester(algID)
		if r == nil {
			continue
		}
		rs[algID] = r
		writers = append(writers, r)
	}
	if len(writers) == 0 {
		return &MultiDigester{Writer: io.Discard}
	}
	return &MultiDigester{
		Writer:    io.MultiWriter(writers...),
		digesters: rs,
	}
}

func (md MultiDigester) Sum(alg string) string {
	if dig := md.digesters[alg]; dig != nil {
		return dig.String()
	}
	return ""
}

// Sums returns a Set with all values for the MultiDigester
func (md MultiDigester) Sums() Set {
	set := make(Set, len(md.digesters))
	for alg, r := range md.digesters {
		set[alg] = r.String()
	}
	return set
}

// Set is a map of alg id to digest values
type Set map[string]string

func (s Set) Add(s2 Set) error {
	for alg, newDigest := range s2 {
		currDigest := s[alg]
		if currDigest == "" {
			s[alg] = newDigest
			continue
		}
		if strings.EqualFold(currDigest, newDigest) {
			continue
		}
		// conflict
		return &DigestError{
			Alg:      alg,
			Got:      newDigest,
			Expected: currDigest,
		}
	}
	return nil
}

// ConflictsWith returns keys in s with values that do not match the
// corresponding key in other. Values in each set are compared using
// strings.EqualFold().
func (s Set) ConflictsWith(other Set) []string {
	var keys []string
	for alg, sv := range s {
		if ov, ok := other[alg]; ok && !strings.EqualFold(sv, ov) {
			keys = append(keys, alg)
		}
	}
	return keys
}

// Validate  reader and return an error if the resulting for any
// algorithm in s doesn't match the value in s.
func (s Set) Validate(reader io.Reader) error {
	algs := make([]string, 0, len(s))
	for alg := range s {
		algs = append(algs, alg)
	}
	r := NewMultiDigester(algs...)
	if _, err := io.Copy(r, reader); err != nil {
		return err
	}
	result := r.Sums()
	for _, alg := range result.ConflictsWith(s) {
		return &DigestError{Alg: alg, Expected: s[alg], Got: result[alg]}
	}
	return nil
}

// DigestError is returned when content's conflicts with an expected value
type DigestError struct {
	Path     string // Content path
	Alg      string // Digest algorithm
	Got      string // Calculated digest
	Expected string // Expected digest
}

func (e DigestError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("unexpected %s value: %q, expected=%q", e.Alg, e.Got, e.Expected)
	}
	return fmt.Sprintf("unexpected %s for %q: %q, expected=%q", e.Alg, e.Path, e.Got, e.Expected)
}
