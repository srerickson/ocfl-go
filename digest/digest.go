package digest

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"hash"

	"golang.org/x/crypto/blake2b"
)

var (
	SHA512  = Alg{id: `sha512`}
	SHA256  = Alg{id: `sha256`}
	SHA224  = Alg{id: `sha224`}
	SHA1    = Alg{id: `sha1`}
	MD5     = Alg{id: `md5`}
	BLAKE2B = Alg{id: `blake2b-512`}

	AlgEmpty = Alg{}
	algs     = map[string]Alg{
		`sha512`:      SHA512,
		`sha256`:      SHA256,
		`sha224`:      SHA224,
		`sha1`:        SHA1,
		`md5`:         MD5,
		`blake2b-512`: BLAKE2B,
	}

	ErrUnknownAlg = errors.New("unsupported digest algorithm")
)

// Alg represents a supported digest algorithm (e.g., "sha512")
type Alg struct {
	id string
}

// Set is a collection of digests for the same content
type Set map[Alg]string

func NewAlg(id string) (Alg, error) {
	alg, ok := algs[id]
	if !ok {
		return Alg{}, fmt.Errorf(`%w: %s`, ErrUnknownAlg, id)
	}
	return alg, nil
}

func (a Alg) New() hash.Hash {
	switch a.id {
	case `sha512`:
		return sha512.New()
	case `sha256`:
		return sha256.New()
	case `sha224`:
		return sha256.New224()
	case `sha1`:
		return sha1.New()
	case `md5`:
		return md5.New()
	case `blake2b-512`:
		return newBlake2b()
	}
	err := fmt.Errorf("%w: '%s'", ErrUnknownAlg, a.id)
	panic(err)
}

func (a Alg) ID() string {
	return a.String()
}

func (a Alg) String() string {
	if _, exists := algs[a.id]; !exists {
		return ""
	}
	return a.id
}

func (a *Alg) UnmarshalText(t []byte) error {
	alg, ok := algs[string(t)]
	if !ok {
		return fmt.Errorf(`%w: %s`, ErrUnknownAlg, alg)
	}
	a.id = alg.id
	return nil
}

func (a Alg) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

// Deprecated
func NewHash(a string) (func() hash.Hash, error) {
	alg, ok := algs[a]
	if !ok {
		return nil, fmt.Errorf(`%w: %s`, ErrUnknownAlg, alg)
	}
	return alg.New, nil
}

func newBlake2b() hash.Hash {
	h, err := blake2b.New512(nil)
	if err != nil {
		panic("cannot create blake2b hash")
	}
	return h
}
