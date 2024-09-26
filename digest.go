package ocfl

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"iter"
	"strings"
	"sync"

	"github.com/srerickson/ocfl-go/internal/pipeline"
	"golang.org/x/crypto/blake2b"
)

var ErrUnknownAlg = errors.New("unknown digest algorithm")

const (
	SHA512  = `sha512`
	SHA256  = `sha256`
	SHA1    = `sha1`
	MD5     = `md5`
	BLAKE2B = `blake2b-512`
)

var (
	// built-in digest algorithm definitions
	builtin = map[string]func() Digester{
		SHA512:  func() Digester { return newHashDigester(sha512.New()) },
		SHA256:  func() Digester { return newHashDigester(sha256.New()) },
		SHA1:    func() Digester { return newHashDigester(sha1.New()) },
		MD5:     func() Digester { return newHashDigester(md5.New()) },
		BLAKE2B: func() Digester { return newHashDigester(mustBlake2bNew512()) },
	}

	// register includes digest algorithms registered with RegisterAlg
	register   = map[string]func() Digester{}
	registerMx = sync.RWMutex{}
)

// RegisteredAlgs returns a slice of all available digest algorithms
func RegisteredAlgs() []string {
	algs := make([]string, 0, len(builtin)+len(register))
	for k := range builtin {
		algs = append(algs, k)
	}
	for k := range register {
		algs = append(algs, k)
	}
	return algs
}

// RegisterAlg registers the Digester constructor for alg, so that alg.New() can
// be used.
func RegisterAlg(alg string, newDigester func() Digester) {
	// check built-in
	if builtin[alg] != nil {
		return
	}
	// check register
	registerMx.Lock()
	defer registerMx.Unlock()
	if register[alg] != nil {
		return
	}
	register[alg] = newDigester
}

// New returns a new Digester for generated digest values. If a Digester
// constructor was not registered for a, nil is returne.
func NewDigester(alg string) Digester {
	// check built-in
	if newDigester := builtin[alg]; newDigester != nil {
		return newDigester()
	}
	// check register
	registerMx.RLock()
	defer registerMx.RUnlock()
	if newDigester := register[alg]; newDigester != nil {
		return newDigester()
	}
	return nil
}

// Digester is an interface used for generating digest values.
type Digester interface {
	io.Writer
	// String() returns the digest value for the bytes written to the digester.
	String() string
}

type hashDigester struct {
	hash.Hash
}

func newHashDigester(h hash.Hash) hashDigester {
	return hashDigester{Hash: h}
}

func (h hashDigester) String() string { return hex.EncodeToString(h.Sum(nil)) }

// MultiDigester is used to generate digests for multiple digest algorithms at
// the same time.
type MultiDigester struct {
	io.Writer
	digesters map[string]Digester
}

func NewMultiDigester(algs ...string) *MultiDigester {
	writers := make([]io.Writer, 0, len(algs))
	digesters := make(map[string]Digester, len(algs))
	for _, alg := range algs {
		if digester := NewDigester(alg); digester != nil {
			digesters[alg] = digester
			writers = append(writers, digester)
		}
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
	Name     string // Content path
	Alg      string // Digest algorithm
	Got      string // Calculated digest
	Expected string // Expected digest
}

func (e DigestError) Error() string {
	if e.Name == "" {
		return fmt.Sprintf("unexpected %s value: %q, expected=%q", e.Alg, e.Got, e.Expected)
	}
	return fmt.Sprintf("unexpected %s for %q: %q, expected=%q", e.Alg, e.Name, e.Got, e.Expected)
}

// Digest concurrently digests files in an FS. The pathAlgs argument is a funcion
// iterator that yields file paths and a slide of digest digest algorithms. It returns an iteratator
// the yields PathDigest or an error.
func Digest(ctx context.Context, fsys FS, pathAlgs iter.Seq2[string, []string]) iter.Seq2[PathDigests, error] {
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
		results := pipeline.Results(jobsIter, runJobs, 1)
		results(func(r pipeline.Result[digestJob, DigestSet]) bool {
			return yield(PathDigests{
				Path:    r.In.path,
				Digests: r.Out,
			}, r.Err)
		})
	}
}

// PathDigests represent on or more computed
// digests for a file in an FS.
type PathDigests struct {
	Path    string
	Digests DigestSet
}

func mustBlake2bNew512() hash.Hash {
	h, err := blake2b.New512(nil)
	if err != nil {
		panic("creating new blake2b hash")
	}
	return h
}
