// Copyright 2019 Seth R. Erickson
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// SHA512 = `sha512`
const (
	SHA512  = `sha512`
	SHA256  = `sha256`
	SHA224  = `sha224`
	SHA1    = `sha1`
	MD5     = `md5`
	BLAKE2B = `blake2b`
)

var digestAlgorithms = [...]string{
	SHA512,
	SHA256,
	SHA224,
	SHA1,
	MD5,
	BLAKE2B,
}

var defaultAlgorithm = SHA512

// NumDigesters sets concurrency for Digest
var NumDigesters = runtime.GOMAXPROCS(0)

type checksumJob struct {
	path     string
	alg      string
	sum      string
	expected string
	err      error
}

// Checksum returns checksum of file at path using algorithm alg
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
		return nil, fmt.Errorf(`unknown checksum algorithm: %s`, alg)
	}
	return h, nil
}

// digester processes checksum jobs concurrently
func digester(ctx context.Context, in <-chan checksumJob) <-chan checksumJob {
	var wg sync.WaitGroup
	out := make(chan checksumJob)
	if NumDigesters < 1 {
		NumDigesters = 1
	}
	wg.Add(NumDigesters)
	for i := 0; i < NumDigesters; i++ {
		go func() {
			defer wg.Done()
			for job := range in {
				select {
				case <-ctx.Done():
					return
				default:
					if job.err == nil {
						job.sum, job.err = Checksum(job.alg, job.path)
					}
					out <- job
				}

			}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// ConcurrentDigest concurrently calculates checksum of every file in dir
// using Hash algorithm alg, returning results as a ContentMap
func ConcurrentDigest(dir string, alg string) (ContentMap, error) {
	var cm ContentMap
	ctx, cancel := context.WithCancel(context.Background())
	jobIn := make(chan checksumJob)
	walkErr := make(chan error, 1)

	// walk files in dir
	go func() {
		defer close(jobIn)
		defer close(walkErr)
		walk := func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				select {
				case jobIn <- checksumJob{path: p, alg: alg}:
				case <-ctx.Done(): // cancel() called
					return errors.New(`walk canceled`)
				}

			}
			return nil
		}
		walkErr <- filepath.Walk(dir, walk)
	}()

	// stream results into ContentMap
	for job := range digester(ctx, jobIn) {
		if job.err != nil {
			cancel() // cancel filepath.Walk
			return cm, job.err
		}
		relPath, _ := filepath.Rel(dir, job.path)
		cm.Add(job.sum, relPath)
	}

	// Handle filepath.Walk errs
	if err := <-walkErr; err != nil {
		cancel()
		return cm, err
	}
	cancel()
	return cm, nil
}
