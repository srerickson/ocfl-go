package digest

import (
	"fmt"
	"io"
	"strings"
)

// Digester is an interface used for generating values.
type Digester interface {
	io.Writer
	// String() returns the digest of the bytes written to the digester.
	String() string
}

// MultiDigester is used to generate digests for multiple algorithms at the same
// time.
type MultiDigester struct {
	io.Writer
	digesters map[string]Digester
}

// NewMultiDigester constructs a MultiDigester for one or more built-in digest
// algorithms.
func NewMultiDigester(algs ...Algorithm) *MultiDigester {
	writers := make([]io.Writer, 0, len(algs))
	digesters := make(map[string]Digester, len(algs))
	for _, alg := range algs {
		digester := alg.Digester()
		digesters[alg.ID()] = digester
		writers = append(writers, digester)
	}
	return &MultiDigester{
		Writer:    io.MultiWriter(writers...),
		digesters: digesters,
	}
}

// Sum returns the digest value for the alg in md.
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

// Algorithms returns the IDs of the algorithms in s.
func (s Set) Algorithms() []string {
	if len(s) == 0 {
		return nil
	}
	algs := make([]string, 0, len(s))
	for alg := range s {
		algs = append(algs, alg)
	}
	return algs
}

// Add adds the digests from s2 to s. An error is returned if there is a conflict.
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

func (s Set) Delete(id string) string {
	val := s[id]
	delete(s, id)
	return val
}

// Validate digests the reader using all algorithms used in s found in reg.
// An error is returned in the resulting digests values conflict with those
// in s.
func (s Set) Validate(r io.Reader, reg AlgorithmRegistry) error {
	digester := NewMultiDigester(reg.GetAny(s.Algorithms()...)...)
	if _, err := io.Copy(digester, r); err != nil {
		return err
	}
	results := digester.Sums()
	for _, alg := range results.ConflictsWith(s) {
		return &DigestError{Alg: alg, Expected: s[alg], Got: results[alg]}
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
