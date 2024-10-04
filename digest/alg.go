package digest

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"

	"golang.org/x/crypto/blake2b"
)

const (
	SHA512  = alg(`sha512`)      // built-in Alg for sha512
	SHA256  = alg(`sha256`)      // built-in Alg for sha256
	SHA1    = alg(`sha1`)        // built-in Alg for sha1
	MD5     = alg(`md5`)         // built-in Alg for md5
	BLAKE2B = alg(`blake2b-512`) // built-in Alg for blake2b
)

// Alg is implemented by digest algorithms
type Alg interface {
	// ID returns the algorithm name (e.g., 'sha512')
	ID() string
	// Digester returns a new digester for generating a new digest value
	Digester() Digester
}

// digester constructors for built-in algs
var builtInDigesters = map[alg]func() Digester{
	SHA512:  func() Digester { return &hashDigester{Hash: sha512.New()} },
	SHA256:  func() Digester { return &hashDigester{Hash: sha256.New()} },
	SHA1:    func() Digester { return &hashDigester{Hash: sha1.New()} },
	MD5:     func() Digester { return &hashDigester{Hash: md5.New()} },
	BLAKE2B: func() Digester { return &hashDigester{Hash: mustBlake2bNew512()} },
}

// alg is a built-in Alg
type alg string

// ID implements Alg for alg
func (a alg) ID() string { return string(a) }

// Digester implements Alg for alg
func (a alg) Digester() Digester { return builtInDigesters[a]() }

// hashDigester implements Digester using a hash.Hash
type hashDigester struct {
	hash.Hash
}

func (h hashDigester) String() string {
	return hex.EncodeToString(h.Sum(nil))
}

func mustBlake2bNew512() hash.Hash {
	h, err := blake2b.New512(nil)
	if err != nil {
		panic("creating new blake2b hash")
	}
	return h
}
