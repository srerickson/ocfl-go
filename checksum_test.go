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

func TestConcurrentDigest(t *testing.T) {

	cm, err := ConcurrentDigest(`test`, `sha1`)
	if err != nil {
		t.Error(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	for err = range cm.Validate(ctx, `test`, `sha1`) {
		t.Error(err)
		break
	}
	cancel()

	// should get error with invalid path
	_, err = ConcurrentDigest(`none`, `sha1`)
	if err == nil {
		t.Error(`expected an error`)
	}

	// should get error with invalid algorithm
	_, err = ConcurrentDigest(`test`, `sha`)
	if err == nil {
		t.Error(`expected an error`)
	}

}
