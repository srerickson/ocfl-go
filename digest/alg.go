package digest

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"io"

	"golang.org/x/crypto/blake2b"
)

const (
	SHA512  = alg(`sha512`)
	SHA256  = alg(`sha256`)
	SHA1    = alg(`sha1`)
	MD5     = alg(`md5`)
	BLAKE2B = alg(`blake2b-512`)
)

var (
	builtin = []Alg{SHA512, SHA256, SHA1, MD5, BLAKE2B}

	// digester constructors for built-in algs
	builtInDigesters = map[alg]func() Digester{
		SHA512:  func() Digester { return &hashDigester{Hash: sha512.New()} },
		SHA256:  func() Digester { return &hashDigester{Hash: sha256.New()} },
		SHA1:    func() Digester { return &hashDigester{Hash: sha1.New()} },
		MD5:     func() Digester { return &hashDigester{Hash: md5.New()} },
		BLAKE2B: func() Digester { return &hashDigester{Hash: mustBlake2bNew512()} },
	}
)

// Alg is implemented by digest algorithms
type Alg interface {
	// ID returns the algorithm name (e.g., 'sha512')
	ID() string
	// Digester returns a new digester for generating a new digest value
	Digester() Digester
}

// Digester is an interface used for generating digest values.
type Digester interface {
	io.Writer
	// String() returns the digest value for the bytes written to the digester.
	String() string
}

// alg is a built-in Alg
type alg string

func (a alg) ID() string { return string(a) }
func (a alg) Digester() Digester {
	if fn := builtInDigesters[a]; fn != nil {
		return fn()
	}
	return nil
}

// hashDigester implements Digester using a hash.Hash
type hashDigester struct {
	hash.Hash
}

func (h hashDigester) String() string { return hex.EncodeToString(h.Sum(nil)) }

func mustBlake2bNew512() hash.Hash {
	h, err := blake2b.New512(nil)
	if err != nil {
		panic("creating new blake2b hash")
	}
	return h
}
