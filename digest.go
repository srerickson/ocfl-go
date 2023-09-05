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

	"github.com/srerickson/ocfl-go/internal/pipeline"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/exp/maps"
)

var ErrUnknownDigestAlg = errors.New("unknown digest algorithm")

var (
	// unspecified digest algorithm
	NOALG = Alg{}

	SHA512  = Alg{`sha512`}
	SHA256  = Alg{`sha256`}
	SHA1    = Alg{`sha1`}
	MD5     = Alg{`md5`}
	BLAKE2B = Alg{`blake2b-512`}

	// Map of built-in Algs
	DigestAlgs = map[string]Alg{
		SHA512.name:  SHA512,
		SHA256.name:  SHA256,
		SHA1.name:    SHA1,
		MD5.name:     MD5,
		BLAKE2B.name: BLAKE2B,
	}
)

// Alg is a built-in digest algorithm. The zero-value is NOALG, an un-specified
// algorithm.
type Alg struct {
	name string
}

// New returns a new Digester for generated digest values.
func (a Alg) New() Digester {
	switch string(a.name) {
	case SHA512.name:
		return newHashDigester(sha512.New())
	case SHA256.name:
		return newHashDigester(sha256.New())
	case SHA1.name:
		return newHashDigester(sha1.New())
	case MD5.name:
		return newHashDigester(md5.New())
	case BLAKE2B.name:
		h, err := blake2b.New512(nil)
		if err != nil {
			panic("creating new blake2b hash")
		}
		return newHashDigester(h)
	default:
		return nil
	}
}

func (a Alg) ID() string { return a.String() }

func (a Alg) String() string {
	return string(a.name)
}

func (a Alg) MarshalText() ([]byte, error) {
	if DigestAlgs[a.name] == NOALG {
		return nil, fmt.Errorf("%q: %w", a.name, ErrUnknownDigestAlg)
	}
	return []byte(a.name), nil
}

func (a *Alg) UnmarshalText(t []byte) error {
	if match, ok := DigestAlgs[string(t)]; ok {
		a.name = match.name
		return nil
	}
	return fmt.Errorf("%q: %w", string(t), ErrUnknownDigestAlg)
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

// MultiDigester is used to generate digests for multiple digest algorithms at the same
// time.
type MultiDigester map[Alg]Digester

func NewMultiDigester(algs ...Alg) MultiDigester {
	md := make(MultiDigester, len(algs))
	for _, alg := range algs {
		md[alg] = alg.New()
	}
	return md
}

// ReadFrom digests r using all algorithms in the MultiDigester.
func (md MultiDigester) ReadFrom(r io.Reader) (int64, error) {
	writers := make([]io.Writer, len(md))
	i := 0
	for _, digester := range md {
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
