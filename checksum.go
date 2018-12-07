package ocfl

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
)

// SHA512 = `sha512`
const (
	SHA512 = `sha512`
	SHA256 = `sha256`
	SHA224 = `sha224`
	SHA1   = `sha1`
	MD5    = `md5`
)

func Checksum(alg string, path string) (string, error) {
	var h hash.Hash
	var err error
	var file io.ReadCloser
	if h, err = newHash(alg); err != nil {
		return ``, err
	}
	if file, err = os.Open(path); err != nil {
		return ``, err
	}
	defer file.Close()
	if _, err = io.Copy(h, file); err != nil {
		return ``, err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// NewHash returns Hash object for specified algorithm
func newHash(alg string) (hash.Hash, error) {
	var h hash.Hash
	switch alg {
	case SHA512:
		h = sha512.New()
	case SHA256:
		h = sha256.New()
	case SHA224:
		h = sha256.New224()
	case SHA1:
		h = sha1.New()
	case MD5:
		h = md5.New()
	default:
		return nil, fmt.Errorf(`Unknown checksum algorithm: %s`, alg)
	}
	return h, nil
}
