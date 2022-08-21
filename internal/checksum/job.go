package checksum

import (
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
)

var ErrNotRegularFile = errors.New(`not a regular file`)

// Job is value streamed to/from Walk and Pool
type Job struct {
	path string                      // path to file
	algs map[string]func() hash.Hash // hash constructor function
	sums map[string][]byte           // checksum result
	err  error                       // any encountered errors
	fs   fs.FS
}

// do does the job
func (j *Job) do() {
	if j.err != nil {
		return
	}
	var file fs.File
	file, j.err = j.fs.Open(j.path)
	if j.err != nil {
		return
	}
	defer file.Close()
	var info fs.FileInfo
	info, j.err = file.Stat()
	if j.err != nil {
		return
	}
	if !info.Mode().IsRegular() {
		j.err = fmt.Errorf(`cannot checksum %s: %w`, j.path, ErrNotRegularFile)
		return
	}
	var hashes = make(map[string]hash.Hash)
	var writers []io.Writer
	for name, newHash := range j.algs {
		h := newHash()
		hashes[name] = h
		writers = append(writers, io.Writer(h))
	}
	multi := io.MultiWriter(writers...)
	_, j.err = io.Copy(multi, file)
	if j.err != nil {
		return
	}
	j.sums = make(map[string][]byte)
	for name, h := range hashes {
		j.sums[name] = h.Sum(nil)
	}
}

// Path returns the Job's path
func (j Job) Path() string {
	return j.path
}

// Sum returns the checksum for the named algorithm. The package defines common
// algorithm names (MD5, SHA256, etc.), otherwise name refers to the string
// passed to WithAlg().
func (j Job) Sum(name string) ([]byte, error) {
	if j.sums == nil || j.sums[name] == nil {
		return nil, fmt.Errorf("checksum %s not found", name)
	}
	var s = make([]byte, len(j.sums[name]))
	copy(s, j.sums[name])
	return s, nil
}

// SumString returns a string representation of the checksum for the named
// algorithm. The package defines common algorithm names (MD5, SHA256, etc.),
// otherwise name refers to the string passed to WithAlg().
func (j Job) SumString(name string) (string, error) {
	s, err := j.Sum(name)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(s), nil
}

// Sums returns map of all checksum calculated by the job
func (j Job) Sums() map[string][]byte {
	ret := make(map[string][]byte)
	for alg := range j.algs {
		ret[alg] = append(make([]byte, 0, len(j.sums[alg])), j.sums[alg]...)
	}
	return ret
}

// Err returns any errors from the Job
func (j Job) Err() error {
	return j.err
}
