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
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
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
		return nil, fmt.Errorf(`Unknown checksum algorithm: %s`, alg)
	}
	return h, nil
}

func digester(in <-chan checksumJob) <-chan checksumJob {
	var wg sync.WaitGroup
	out := make(chan checksumJob)
	for i := 0; i < NumDigesters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range in {
				if job.err == nil {
					job.sum, job.err = Checksum(job.alg, job.path)
				}
				out <- job
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
	jobIn := make(chan checksumJob)
	walkErr := make(chan error)
	// input queue
	go func() {
		walk := func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				jobIn <- checksumJob{path: p, alg: alg}
			}
			return nil
		}
		err := filepath.Walk(dir, walk)
		close(jobIn)
		if err != nil {
			walkErr <- err
		}
	}()

	var lastErr error
	for job := range digester(jobIn) {
		if job.err != nil {
			lastErr = job.err
		} else {
			relPath, _ := filepath.Rel(dir, job.path)
			cm.Add(job.sum, relPath)
		}
	}

	select {
	case lastErr = <-walkErr:
	default:
	}

	return cm, lastErr
}

// ValidateHandleErr confirms digests in ContentMap using hash algorithm alg and
// dir as a base path for relative paths in the ContentMap
func (cm *ContentMap) ValidateHandleErr(dir string, alg string, handle func(error)) error {
	in := make(chan checksumJob)
	go func() {
		for dp := range cm.Iterate() {
			in <- checksumJob{
				path:     filepath.Join(dir, string(dp.Path)),
				alg:      alg,
				expected: string(dp.Digest),
			}
		}
		close(in)
	}()
	var lastErr error
	for result := range digester(in) {
		if result.err != nil {
			lastErr = result.err
			if handle != nil {
				handle(lastErr)
			}
			continue
		}
		if result.sum != result.expected {
			lastErr = fmt.Errorf(`checksum failed: %s`, result.path)
			if handle != nil {
				handle(lastErr)
			}
		}
	}
	return lastErr
}
