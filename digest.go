package ocfl

import (
	"context"
	"errors"
	"io"
	"iter"
	"runtime"

	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/internal/pipeline"
)

// Digest is equivalent to ConcurrentDigest with the number of digest workers
// set to runtime.NumCPU(). The pathAlgs argument is an iterator that yields
// file paths and a slice of digest algorithms. It returns an iteratator the
// yields PathDigest or an error.
func Digest(ctx context.Context, fsys FS, pathAlgs iter.Seq2[string, []digest.Algorithm]) iter.Seq2[PathDigests, error] {
	return ConcurrentDigest(ctx, fsys, pathAlgs, runtime.NumCPU())
}

// ConcurrentDigest concurrently digests files in an FS. The pathAlgs argument
// is an iterator that yields file paths and a slice of digest algorithms. It
// returns an iteratator the yields PathDigest or an error.
func ConcurrentDigest(ctx context.Context, fsys FS, pathAlgs iter.Seq2[string, []digest.Algorithm], numWorkers int) iter.Seq2[PathDigests, error] {
	// checksum digestJob
	type digestJob struct {
		path string
		algs []digest.Algorithm
	}
	jobsIter := func(yield func(digestJob) bool) {
		pathAlgs(func(name string, algs []digest.Algorithm) bool {
			return yield(digestJob{path: name, algs: algs})
		})
	}
	runJobs := func(j digestJob) (digests digest.Set, err error) {
		f, err := fsys.OpenFile(ctx, j.path)
		if err != nil {
			return
		}
		defer func() {
			if closeErr := f.Close(); closeErr != nil {
				err = errors.Join(err, closeErr)
			}
		}()
		digester := digest.NewMultiDigester(j.algs...)
		if _, err = io.Copy(digester, f); err != nil {
			return
		}
		digests = digester.Sums()
		return
	}
	return func(yield func(PathDigests, error) bool) {
		for result := range pipeline.Results(jobsIter, runJobs, numWorkers) {
			pd := PathDigests{
				Path:    result.In.path,
				Digests: result.Out,
			}
			if !yield(pd, result.Err) {
				break
			}
		}
	}
}

// PathDigests represent on or more computed
// digests for a file in an FS.
type PathDigests struct {
	Path    string
	Digests digest.Set
}
