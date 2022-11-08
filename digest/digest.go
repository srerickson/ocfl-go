package digest

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"hash"

	"golang.org/x/crypto/blake2b"
)

const (
	SHA512id  = `sha512`
	SHA256id  = `sha256`
	SHA224id  = `sha224`
	SHA1id    = `sha1`
	MD5id     = `md5`
	BLAKE2Bid = `blake2b-512`
)

type Alg interface {
	ID() string
	New() hash.Hash
}

// Set is a set of digest results
type Set map[string]string

// builtin algorithms
func SHA512() Alg  { return algSHA512{} }
func SHA256() Alg  { return algSHA256{} }
func SHA224() Alg  { return algSHA224{} }
func SHA1() Alg    { return algSHA1{} }
func MD5() Alg     { return algMD5{} }
func BLAKE2B() Alg { return algBlake2B512{} }

type algSHA512 struct{}

func (alg algSHA512) ID() string     { return SHA512id }
func (alg algSHA512) New() hash.Hash { return sha512.New() }

type algSHA256 struct{}

func (alg algSHA256) ID() string     { return SHA256id }
func (alg algSHA256) New() hash.Hash { return sha256.New() }

type algSHA224 struct{}

func (alg algSHA224) ID() string     { return SHA224id }
func (alg algSHA224) New() hash.Hash { return sha512.New512_224() }

type algSHA1 struct{}

func (alg algSHA1) ID() string     { return SHA1id }
func (alg algSHA1) New() hash.Hash { return sha1.New() }

type algMD5 struct{}

func (alg algMD5) ID() string     { return MD5id }
func (alg algMD5) New() hash.Hash { return md5.New() }

type algBlake2B512 struct{}

func (alg algBlake2B512) ID() string { return BLAKE2Bid }
func (alg algBlake2B512) New() hash.Hash {
	h, err := blake2b.New512(nil)
	if err != nil {
		panic("cannot create blake2b hash")
	}
	return h
}
