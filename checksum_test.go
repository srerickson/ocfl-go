package ocfl

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCancelDigester(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	jobs := make(chan checksumJob, 1)
	results := digester(ctx, jobs)
	job := checksumJob{
		path: filepath.Join(`test`, `fixtures`, `README.md`),
		alg:  SHA1,
	}
	// this job should work
	jobs <- job
	r := <-results
	if r.sum == `` {
		t.Errorf(`expected value, got %v`, r.err)
	}

	// cancel the context
	cancel()

	// this job should not
	jobs <- job
	select {
	case <-results:
		t.Error(`don't expect a result`)
	case <-ctx.Done():

	}

}
