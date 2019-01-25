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
	r = <-results
	if r.sum != `` || r.err == nil {
		t.Errorf(`expected no sum and an error, but got %s`, r.sum)
	}
}

func TestConcurrentDigest(t *testing.T) {

	cm, err := ConcurrentDigest(`test`, `sha1`)
	if err != nil {
		t.Error(err)
	}

	// FIXME: clean this mess up
	ctx := context.Background()
	var v Validator
	v.errChan = make(chan error)
	v.root = `test`
	var checked int
	go func() {
		checked = v.validateContentMap(ctx, cm, `sha1`)
		close(v.errChan)
	}()

	for e := range v.errChan {
		err = e
	}
	if err != nil {
		t.Error(err)
	}
	if cm.Len() != checked {
		t.Errorf(`expected %d files to be checked`, checked)
	}

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
