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
	"strings"
	"sync"

	"github.com/srerickson/ocfl-go/internal/pipeline"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/exp/maps"
)

var ErrUnknownAlg = errors.New("unknown digest algorithm")

const (
	NOALG = Alg("") // unspecified digest algorithm

	SHA512  = Alg(`sha512`)
	SHA256  = Alg(`sha256`)
	SHA1    = Alg(`sha1`)
	MD5     = Alg(`md5`)
	BLAKE2B = Alg(`blake2b-512`)
)

var (
	// global alg register
	register = map[Alg]func() Digester{
		SHA512:  func() Digester { return newHashDigester(sha512.New()) },
		SHA256:  func() Digester { return newHashDigester(sha256.New()) },
		SHA1:    func() Digester { return newHashDigester(sha1.New()) },
		MD5:     func() Digester { return newHashDigester(md5.New()) },
		BLAKE2B: func() Digester { return newHashDigester(mustBlake2bNew512()) },
	}
	registerMx = sync.RWMutex{}
)

// DefaultAlgs returns slice of built-in Algs.
func DefaultAlgs() []Alg {
	return []Alg{SHA512, SHA256, SHA1, MD5, BLAKE2B}
}

// RegisterAlg registers the Digester constructor for alg, so that alg.New() can
// be used.
func RegisterAlg(alg Alg, newDigester func() Digester) {
	registerMx.Lock()
	defer registerMx.Unlock()
	if register[alg] != nil {
		return
	}
	register[alg] = newDigester
}

// Alg is a built-in digest algorithm. The zero-value is NOALG, an un-specified
// algorithm.
type Alg string

// New returns a new Digester for generated digest values. If a Digester
// constructor was not registered for a, nil is returne.
func (a Alg) New() Digester {
	registerMx.RLock()
	defer registerMx.RUnlock()
	if newDigester := register[a]; newDigester != nil {
		return newDigester()
	}
	return nil
}

// ID returns the Alg's name
func (a Alg) ID() string { return string(a) }

// String() implements the stringer interface for Alg. It returns the Alg's
// name.
func (a Alg) String() string { return string(a) }

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
type MultiDigester map[Alg]Digester

func NewMultiDigester(algs ...Alg) MultiDigester {
	md := make(MultiDigester, len(algs))
	for _, alg := range algs {
		if digester := alg.New(); digester != nil {
			md[alg] = digester
		}
	}
	return md
}

// ReadFrom digests r using all algorithms in the MultiDigester.
func (md MultiDigester) ReadFrom(r io.Reader) (int64, error) {
	writers := make([]io.Writer, len(md))
	i := 0
	for _, digester := range md {
		if digester == nil {
			continue
		}
		writers[i] = digester
		i++
	}
	return io.Copy(io.MultiWriter(writers...), r)
}

// Sums returns a DigestSet with all digest values
// for the MultiDigester
func (md MultiDigester) Sums() DigestSet {
	set := make(DigestSet, len(md))
	for alg, digester := range md {
		set[alg] = digester.String()
	}
	return set
}

// Set is a set of digest results
type DigestSet map[Alg]string

// Validate digests reader and return an error if the resulting digest for any
// algorithm in s doesn't match the value in s.
func (s DigestSet) Validate(reader io.Reader) (err error) {
	digester := NewMultiDigester(maps.Keys(s)...)
	if _, err = digester.ReadFrom(reader); err != nil {
		return err
	}
	result := digester.Sums()
	conflicts := result.ConflictWith(s)
	for _, a := range conflicts {
		err = errors.Join(err, &DigestErr{Alg: a, Expected: s[a], Got: result[a]})
	}
	return err
}

// ConflictWith returns keys in s with values that do not match the corresponding
// key in other.
func (s DigestSet) ConflictWith(other DigestSet) []Alg {
	var keys []Alg
	for alg, sv := range s {
		if ov, ok := other[alg]; ok && !strings.EqualFold(sv, ov) {
			keys = append(keys, alg)
		}
	}
	return keys
}

// DigestErr is returned when content's digest conflicts with an expected value
type DigestErr struct {
	Name     string // Content path
	Alg      Alg    // Digest algorithm
	Got      string // Calculated digest
	Expected string // Expected digest
}

func (e DigestErr) Error() string {
	if e.Name == "" {
		return fmt.Sprintf("unexpected %s value: %q, expected=%q", e.Alg, e.Got, e.Expected)
	}
	return fmt.Sprintf("unexpected %s for %q: %q, expected=%q", e.Alg, e.Name, e.Got, e.Expected)
}

// DigestFS concurrently digests files in an FS. The setup function adds files
// to the work quue using the addFile function passed to it. addFile returns a
// bool indicating if the file was added to the queue. Results are passed back
// using the result function. If resultFn returns an error, not more results
// will be produced, and new calls to addFile will return false. DigestFS uses
// the value from DigestConcurrency() to determine to set the number of files
// that are digested concurrently.
func DigestFS(ctx context.Context, fsys FS, setupFunc func(addFile func(string, ...Alg) bool), resultFn func(string, DigestSet, error) error) error {
	addJobs := func(addJob func(digestJob) bool) {
		addFileJob := func(name string, algs ...Alg) bool {
			return addJob(digestJob{path: name, algs: algs})
		}
		setupFunc(addFileJob)
	}
	runJobs := func(j digestJob) (DigestSet, error) {
		f, err := fsys.OpenFile(ctx, j.path)
		if err != nil {
			return nil, err
		}
		if closer, ok := f.(io.Closer); ok {
			defer closer.Close()
		}
		digester := NewMultiDigester(j.algs...)
		if _, err = digester.ReadFrom(f); err != nil {
			return nil, err
		}
		return digester.Sums(), nil
	}

	returnJobs := func(j digestJob, sums DigestSet, err error) error {
		return resultFn(j.path, sums, err)
	}
	return pipeline.Run(addJobs, runJobs, returnJobs, DigestConcurrency())
}

// checksum digestJob
type digestJob struct {
	path string
	algs []Alg
}

func mustBlake2bNew512() hash.Hash {
	h, err := blake2b.New512(nil)
	if err != nil {
		panic("creating new blake2b hash")
	}
	return h
}
