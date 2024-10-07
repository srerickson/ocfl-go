package extension

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"strconv"

	"github.com/srerickson/ocfl-go/digest"
	"golang.org/x/crypto/blake2b"
)

// Algorithm is a digest.Algorithm provided by an extension
type Algorithm interface {
	digest.Algorithm
	// Extension returns the AlgorithRegistry extension that provides the
	// algorithm.
	Extension() AlgorithmRegistry
}

// AlgorithmRegistry is an extension that provides a registry of digest
// algorithms
type AlgorithmRegistry interface {
	Extension
	Algorithms() digest.Registry
}

// algRegistry is an implementation of AlgorithmRegistry
type algRegistry struct {
	Base
	algs digest.Registry
}

// Algorithms implements DigestAlgorithms for digestAlgorithms
func (d algRegistry) Algorithms() digest.Registry { return d.algs }

// alg is an implementation of Algorithm used by all extension digest algorithms
type alg struct {
	id  string // digest algorithm's id ("size")
	ext AlgorithmRegistry
}

func (a alg) ID() string { return a.id }

func (a alg) Digester() digest.Digester {
	switch a.id {
	case "blake2b-160":
		return &hashDigester{Hash: mustNewBlake2B(20)}
	case "blake2b-256":
		return &hashDigester{Hash: mustNewBlake2B(32)}
	case "blake2b-384":
		return &hashDigester{Hash: mustNewBlake2B(48)}
	case "sha512/256":
		return &hashDigester{Hash: sha512.New512_256()}
	case "size":
		return &sizeDigester{}
	default:
		panic("invalid algorithm id: " + a.id)
	}
}

// hashDigester implements Digester using a hash.Hash
type hashDigester struct {
	hash.Hash
}

func (h hashDigester) String() string {
	return hex.EncodeToString(h.Sum(nil))
}

// sizeDigester implements a Digester that counts bytes
type sizeDigester struct {
	size int64
}

func (d *sizeDigester) Write(b []byte) (int, error) {
	l := len(b)
	d.size += int64(l)
	return l, nil
}

func (d *sizeDigester) String() string {
	return strconv.FormatInt(d.size, 10)
}

func getHash(name string) hash.Hash {
	switch name {
	case `sha512`:
		return sha512.New()
	case `sha256`:
		return sha256.New()
	case `sha1`:
		return sha1.New()
	case `md5`:
		return md5.New()
	case `blake2b-512`:
		return mustNewBlake2B(64)
	default:
		return nil
	}
}

func mustNewBlake2B(size int) hash.Hash {
	h, err := blake2b.New(size, nil)
	if err != nil {
		panic("creating new blake2b hash")
	}
	return h
}
