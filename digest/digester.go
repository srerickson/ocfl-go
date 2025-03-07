package digest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"

	"github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/internal/pipeline"
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

// Split returns the digest value for algID and Set with remaining values in s.
// This is mainly used to separate the primary digest from 'fixity' digests.
func (s Set) Split(algID string) (digest string, fixity Set) {
	digest = s[algID]
	fixity = Set{}
	for alg, val := range s {
		if alg == algID {
			continue
		}
		fixity[alg] = val
	}
	return
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

// DigestError is returned when content's conflicts with an expected value
type DigestError struct {
	Path     string // Content path
	Alg      string // Digest algorithm
	Got      string // Calculated digest
	Expected string // Expected digest
}

// Error() makes DigestError an error
func (e DigestError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("unexpected %s value: %q, expected=%q", e.Alg, e.Got, e.Expected)
	}
	return fmt.Sprintf("unexpected %s for %q: %q, expected=%q", e.Alg, e.Path, e.Got, e.Expected)
}

// FileRef is a [fs.FileRef] plus digest values of the file contents.
type FileRef struct {
	fs.FileRef
	Algorithm Algorithm // primary digest algorithm (sha512 or sha256)
	Digests   Set       // Digest values (must include the primary algorithm)
}

// DigestFiles is the same as [DigestFilesBatch] with numgos set to 1.
func DigestFiles(ctx context.Context, files iter.Seq[*fs.FileRef], alg Algorithm, fixAlgs ...Algorithm) iter.Seq2[*FileRef, error] {
	return DigestFilesBatch(ctx, files, 1, alg, fixAlgs...)
}

// DigestFilesBatch concurrently computes digests for each file in files. The
// resulting iterator yields digest results or an error if the file could not be
// digestsed. If numgos is < 1, the value from [runtime.GOMAXPROCS](0) is used.
func DigestFilesBatch(ctx context.Context, files iter.Seq[*fs.FileRef], numgos int, alg Algorithm, fixityAlgs ...Algorithm) iter.Seq2[*FileRef, error] {
	algs := make([]Algorithm, 1+len(fixityAlgs))
	algs[0] = alg
	for i := 0; i < len(fixityAlgs); i++ {
		algs[i+1] = fixityAlgs[i]
	}
	digestFn := func(ref *fs.FileRef) (*FileRef, error) {
		f, err := ref.Open(ctx)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		digester := NewMultiDigester(algs...)
		if _, err = io.Copy(digester, f); err != nil {
			return nil, fmt.Errorf("digesting %s: %w", ref.FullPath(), err)
		}
		fd := &FileRef{
			FileRef:   *ref,
			Algorithm: alg,
			Digests:   digester.Sums(),
		}
		return fd, nil
	}
	return func(yield func(*FileRef, error) bool) {
		for result := range pipeline.Results(iter.Seq[*fs.FileRef](files), digestFn, numgos) {
			if !yield(result.Out, result.Err) {
				break
			}
		}
	}
}

// ValidateFilesBatch concurrently validates file digests is digests using
// numgos go routines. It returns an iterator of error values for failed
// validations validation. If validation fails because a file's content has
// changed, the yielded error is a *[DigestError].
func ValidateFilesBatch(ctx context.Context, digests iter.Seq[*FileRef], reg AlgorithmRegistry, numgos int) iter.Seq[error] {
	doDigest := func(pd *FileRef) (*FileRef, error) {
		return pd, validateFile(ctx, pd, reg)
	}
	return func(yield func(error) bool) {
		for result := range pipeline.Results(digests, doDigest, numgos) {
			if result.Err != nil {
				if !yield(result.Err) {
					break
				}
			}
		}
	}
}

// Validate digests the reader using all algorithms in s found in reg.
// An error is returned in the resulting digests values conflict with those
// in s.
func Validate(r io.Reader, s Set, reg AlgorithmRegistry) error {
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

// validateFile confirms the digest values in pd using alogirthm definitions from
// reg. If the digests values do not match, the resulting error is a
// *[DigestError].
func validateFile(ctx context.Context, fileSums *FileRef, reg AlgorithmRegistry) error {
	f, err := fileSums.Open(ctx)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := Validate(f, fileSums.Digests, reg); err != nil {
		var digestErr *DigestError
		if errors.As(err, &digestErr) {
			digestErr.Path = fileSums.FullPath()
		}
		return err
	}
	return nil
}
