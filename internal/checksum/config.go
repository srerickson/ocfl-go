package checksum

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"hash"
	"io/fs"
	"runtime"
)

const (
	MD5    = `md5`
	SHA1   = `sha1`
	SHA512 = `sha512`
	SHA256 = `sha256`
	//BLAKE2B512 = `blake2b-512`
)

// Config is a common configuration object
// used by Walk(), NewPipe(), and Add().
type Config struct {
	numGos      int // number of goroutines in pool
	ctx         context.Context
	algs        map[string]func() hash.Hash
	walkDirFunc fs.WalkDirFunc
}

func defaultConfig() Config {
	return Config{
		numGos:      runtime.GOMAXPROCS(0),
		ctx:         context.Background(),
		walkDirFunc: DefaultWalkDirFunc,
	}
}

// withConfig is used internally to set the whole
// config object if necessary
func withConfig(newC *Config) func(c *Config) {
	return func(oldC *Config) {
		*oldC = *newC
	}
}

// WithGos is used set the number of goroutines used by a Pipe.
// Used as an optional argument for NewPipe().
func WithGos(n int) func(*Config) {
	return func(c *Config) {
		if n < 1 {
			n = 1
		}
		c.numGos = n
	}
}

// WithCtx sets a context for Walk() and NewPipe().
func WithCtx(ctx context.Context) func(*Config) {
	return func(c *Config) {
		c.ctx = ctx
	}
}

// WithAlg adds the named algorith to Walk() and NewPipe().
// Can be repeated for different Algs.
func WithAlg(name string, alg func() hash.Hash) func(*Config) {
	return func(c *Config) {
		if c.algs == nil {
			c.algs = make(map[string]func() hash.Hash)
		}
		c.algs[name] = alg
	}
}

// WithMD5 adds the md5 algorith to Walk() and NewPipe().
func WithMD5() func(*Config) {
	return func(c *Config) {
		WithAlg(MD5, md5.New)(c)
	}
}

// WithSHA1 adds the sha1 algorith to Walk() and NewPipe().
func WithSHA1() func(*Config) {
	return func(c *Config) {
		WithAlg(SHA1, sha1.New)(c)
	}
}

// WithSHA256 adds the sha256 algorith to Walk() and NewPipe().
func WithSHA256() func(*Config) {
	return func(c *Config) {
		WithAlg(SHA256, sha256.New)(c)
	}
}

// WithSHA512 adds the sha512 algorith to Walk() and NewPipe().
func WithSHA512() func(*Config) {
	return func(c *Config) {
		WithAlg(SHA512, sha512.New)(c)
	}
}

// WithWalkDirFunc configures the WalkDirFunc use by Walk().
// It behaves like fs.WalkDirFunc with the addition that
// returning SkipFile causes the file to not be added to the
// Pipe. Has no effect when used with NewPipe().
func WithWalkDirFunc(f fs.WalkDirFunc) func(*Config) {
	return func(c *Config) {
		c.walkDirFunc = f
	}
}
