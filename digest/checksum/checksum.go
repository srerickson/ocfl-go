package checksum

import (
	"context"
	"io"
	"io/fs"
	"os"
	"runtime"

	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/internal/pipeline"
)

// checksum job
type job struct {
	path string       // path to file
	algs []digest.Alg // hash name -> hash constructor
}

// checksum result
type result struct {
	path string
	sums digest.Set
	err  error
}

type checksum struct {
	openFunc func(string) (io.Reader, error)
	fs       fs.FS
	numGos   int
	progress io.Writer
	algs     map[string]digest.Alg
}

// Run does concurrent checksumming. setupFN is a callback used to setup the
// digest pipeline. It takes a function that is used to add path names and
// (optionally) digest algorithms to the pipeline. resultFN is another callback
// used to pass result back (it runs in the same go routine as as Run()).
func Run(ctx context.Context,
	setupFn func(func(name string, algs ...digest.Alg) error) error,
	resultFn func(name string, result digest.Set, err error) error,
	opts ...Option) error {

	checksum := &checksum{
		numGos: runtime.NumCPU(),
		openFunc: func(name string) (io.Reader, error) {
			return os.Open(name)
		},
	}
	for _, o := range opts {
		o(checksum)
	}
	if checksum.fs != nil {
		checksum.openFunc = func(name string) (io.Reader, error) {
			return checksum.fs.Open(name)
		}
	}
	pipeSetup := func(add func(j job) error) error {
		return setupFn(func(name string, algs ...digest.Alg) error {
			return add(job{path: name, algs: algs})
		})
	}
	pipeResult := func(r result) error {
		return resultFn(r.path, r.sums, r.err)
	}
	return pipeline.Run(
		ctx,
		pipeSetup,
		checksum.runFunc(),
		pipeResult,
		checksum.numGos)
}

type Option func(*checksum)

// WithFS is a functional option used to set an FS backend for the checksum.
func WithFS(fsys fs.FS) Option {
	return func(c *checksum) {
		c.fs = fsys
	}
}

// WithOpenFunc is a functional options to set a function used to open filenames
// passed to the checksum process
func WithOpenFunc(open func(string) (io.Reader, error)) Option {
	return func(c *checksum) {
		c.openFunc = open
	}
}

// WithNumGos sets the number of goroutines dedicated to processing checksums.
// Defaults to 1.
func WithNumGos(gos int) Option {
	return func(c *checksum) {
		c.numGos = gos
	}
}

func WithProgress(w io.Writer) Option {
	return func(c *checksum) {
		c.progress = w
	}
}

func WithAlgs(algs ...digest.Alg) Option {
	return func(c *checksum) {
		if c.algs == nil {
			c.algs = make(map[string]digest.Alg, len(algs))
		}
		for _, alg := range algs {
			c.algs[alg.ID()] = alg
		}
	}
}

func (ch *checksum) runFunc() func(context.Context, job) (result, error) {
	return func(ctx context.Context, j job) (result, error) {
		r := result{path: j.path}
		f, err := ch.openFunc(j.path)
		if err != nil {
			return r, err
		}
		if closer, ok := f.(io.Closer); ok {
			defer func() {
				err := closer.Close()
				if err != nil && r.err == nil {
					r.err = err
				}
			}()
		}
		algs := ch.mergeJobAlgs(j.algs)
		dig := digest.NewDigester(algs...)
		if ch.progress != nil {
			f = io.TeeReader(f, ch.progress)
		}
		_, r.err = dig.ReadFrom(f)
		if r.err == nil {
			r.sums = dig.Sums()
		}
		return r, r.err
	}
}

// merge algs configured as option to Run with algs
// passes with thejob.
func (ch checksum) mergeJobAlgs(jobAlgs []digest.Alg) []digest.Alg {
	if len(ch.algs) == 0 {
		return jobAlgs
	}
	newAlgs := make([]digest.Alg, len(ch.algs))
	i := 0
	for _, a := range ch.algs {
		newAlgs[i] = a
		i++
	}
	for _, a := range jobAlgs {
		if _, ok := ch.algs[a.ID()]; !ok {
			newAlgs = append(newAlgs, a)
		}
	}
	return newAlgs
}
