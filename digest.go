package ocfl

import (
	"context"
	"errors"
	"fmt"
	"hash"
	"io"
	"iter"
	"path"
	"runtime"
	"strings"

	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/internal/pipeline"
	"golang.org/x/crypto/blake2b"
)

// MultiDigester is used to generate digests for multiple digest algorithms at
// the same time.
type MultiDigester struct {
	io.Writer
	digesters map[string]digest.Digester
}

func NewMultiDigester(algs ...string) *MultiDigester {
	writers := make([]io.Writer, 0, len(algs))
	digesters := make(map[string]digest.Digester, len(algs))
	for _, algID := range algs {
		alg, _ := digest.Defaults.New(algID)
		if alg == nil {
			continue
		}
		digester := alg.Digester()
		digesters[alg.ID()] = digester
		writers = append(writers, digester)
	}
	if len(writers) == 0 {
		return &MultiDigester{Writer: io.Discard}
	}
	return &MultiDigester{
		Writer:    io.MultiWriter(writers...),
		digesters: digesters,
	}
}

func (md MultiDigester) Sum(alg string) string {
	if dig := md.digesters[alg]; dig != nil {
		return dig.String()
	}
	return ""
}

// Sums returns a DigestSet with all digest values
// for the MultiDigester
func (md MultiDigester) Sums() DigestSet {
	set := make(DigestSet, len(md.digesters))
	for alg, digester := range md.digesters {
		set[alg] = digester.String()
	}
	return set
}

// Set is a set of digest results
type DigestSet map[string]string

func (s DigestSet) Add(s2 DigestSet) error {
	for alg, newDigest := range s2 {
		currDigest := s[alg]
		if currDigest == "" {
			s[alg] = newDigest
			continue
		}
		if strings.EqualFold(currDigest, newDigest) {
			continue
		}
		// digest conflict
		return &DigestError{
			Alg:      alg,
			Got:      newDigest,
			Expected: currDigest,
		}
	}
	return nil
}

// ConflictWith returns keys in s with values that do not match the corresponding
// key in other.
func (s DigestSet) ConflictWith(other DigestSet) []string {
	var keys []string
	for alg, sv := range s {
		if ov, ok := other[alg]; ok && !strings.EqualFold(sv, ov) {
			keys = append(keys, alg)
		}
	}
	return keys
}

// Validate digests reader and return an error if the resulting digest for any
// algorithm in s doesn't match the value in s.
func (s DigestSet) Validate(reader io.Reader) error {
	algs := make([]string, 0, len(s))
	for alg := range s {
		algs = append(algs, alg)
	}
	digester := NewMultiDigester(algs...)
	if _, err := io.Copy(digester, reader); err != nil {
		return err
	}
	result := digester.Sums()
	for _, alg := range result.ConflictWith(s) {
		return &DigestError{Alg: alg, Expected: s[alg], Got: result[alg]}
	}
	return nil
}

// DigestError is returned when content's digest conflicts with an expected value
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

// Digest is equivalent to ConcurrentDigest with the number of digest workers
// set to runtime.NumCPU(). The pathAlgs argument is an iterator that yields
// file paths and a slice of digest algorithms. It returns an iteratator the
// yields PathDigest or an error.
func Digest(ctx context.Context, fsys FS, pathAlgs iter.Seq2[string, []string]) iter.Seq2[PathDigests, error] {
	return ConcurrentDigest(ctx, fsys, pathAlgs, runtime.NumCPU())
}

// ConcurrentDigest concurrently digests files in an FS. The pathAlgs argument
// is an iterator that yields file paths and a slice of digest algorithms. It
// returns an iteratator the yields PathDigest or an error.
func ConcurrentDigest(ctx context.Context, fsys FS, pathAlgs iter.Seq2[string, []string], numWorkers int) iter.Seq2[PathDigests, error] {
	// checksum digestJob
	type digestJob struct {
		path string
		algs []string
	}
	jobsIter := func(yield func(digestJob) bool) {
		pathAlgs(func(name string, algs []string) bool {
			return yield(digestJob{path: name, algs: algs})
		})
	}
	runJobs := func(j digestJob) (digests DigestSet, err error) {
		f, err := fsys.OpenFile(ctx, j.path)
		if err != nil {
			return
		}
		defer func() {
			if closeErr := f.Close(); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
		}()
		digester := NewMultiDigester(j.algs...)
		if _, err = io.Copy(digester, f); err != nil {
			return
		}
		digests = digester.Sums()
		return
	}
	return func(yield func(PathDigests, error) bool) {
		for result := range pipeline.Results(jobsIter, runJobs, numWorkers) {
			pd := PathDigests{
				Path:    result.In.path,
				Digests: result.Out,
			}
			if !yield(pd, result.Err) {
				break
			}
		}
	}
}

// PathDigests represent on or more computed
// digests for a file in an FS.
type PathDigests struct {
	Path    string
	Digests DigestSet
}

// Validate validates pd's DigestSet by reading the file at pd.Path, relative to
// the directory parent in fsys. The returned bool is true if the file was read
// and all digests validated; in this case, the returned error may be non-nil if
// error occured closing the file.
func (pd PathDigests) Validate(ctx context.Context, fsys FS, parent string) (bool, error) {
	f, err := fsys.OpenFile(ctx, path.Join(parent, pd.Path))
	if err != nil {
		return false, err
	}
	if err := pd.Digests.Validate(f); err != nil {
		f.Close()
		var digestErr *DigestError
		if errors.As(err, &digestErr) {
			digestErr.Path = pd.Path
		}
		return false, err
	}
	return true, f.Close()
}

func mustBlake2bNew512() hash.Hash {
	h, err := blake2b.New512(nil)
	if err != nil {
		panic("creating new blake2b hash")
	}
	return h
}
