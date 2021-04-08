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
	"errors"
	"fmt"
	"hash"
	"io/fs"
	"runtime"

	"github.com/srerickson/checksum"
	"golang.org/x/crypto/blake2b"
)

// SHA512 = `sha512`
const (
	SHA512  = `sha512`
	SHA256  = `sha256`
	SHA224  = `sha224`
	SHA1    = `sha1`
	MD5     = `md5`
	BLAKE2B = `blake2b-512`
)

var digestAlgorithms = [...]string{
	SHA512,
	SHA256,
	SHA224,
	SHA1,
	MD5,
	BLAKE2B,
}

//var defaultAlgorithm = SHA512

func newHash(alg string) (func() hash.Hash, error) {
	switch alg {
	case SHA512:
		return sha512.New, nil
	case SHA256:
		return sha256.New, nil
	case SHA1:
		return sha1.New, nil
	case MD5:
		return md5.New, nil
	case BLAKE2B:
		h, err := blake2b.New512(nil)
		if err != nil {
			return nil, err
		}
		return func() hash.Hash {
			return h
		}, nil
	}
	return nil, fmt.Errorf(`unknown checksum algorithm: %s`, alg)
}

// NumDigesters sets concurrency for Digest
var NumDigesters = runtime.GOMAXPROCS(0)

// FSContentMap concurrently calculates checksum of every file in dir
// using Hash algorithm alg, returning results as a ContentMap
func FSContentMap(fsys fs.FS, root string, alg string) (DigestMap, error) {
	var cm DigestMap
	newH, err := newHash(alg)
	if err != nil {
		return nil, err
	}
	each := func(j checksum.Job, err error) error {
		if err != nil {
			return err
		}
		sum, err := j.SumString(alg)
		// fmt.Println(j.Path())
		if err != nil {
			return err
		}
		return cm.Add(sum, j.Path())
	}
	err = checksum.Walk(fsys, root, each, checksum.WithAlg(alg, newH))
	if err != nil {
		walkErr, _ := err.(*checksum.WalkErr)
		// if the walk failed because the dir doesn't exists
		// return that error
		if errors.Is(walkErr.WalkDirErr, fs.ErrNotExist) {
			return nil, walkErr.WalkDirErr
		}
		return nil, err
	}
	return cm, nil
}
