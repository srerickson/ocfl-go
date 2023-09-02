package pipeline_test

import (
	"errors"
	"testing"

	"github.com/srerickson/ocfl-go/internal/pipeline"
)

type job int
type result int

func run(j job) (result, error) {
	var r result
	if j < 0 {
		return r, errors.New("invalid value")
	}
	r = result(j * 2)
	return r, nil
}

func TestPipelineNil(t *testing.T) {
	err := pipeline.Run[job, result](nil, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPipelineResultErr(t *testing.T) {
	input := func(add func(job) bool) {
		add(job(-1)) // invalid job
	}
	output := func(in job, out result, err error) error {
		return err
	}
	err := pipeline.Run(input, run, output, 0)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestPipeline(t *testing.T) {
	times := 100
	input := func(add func(job) bool) {
		for i := 0; i < times; i++ {
			add(job(i))
		}
	}
	var results int
	output := func(in job, out result, err error) error {
		results++
		return nil
	}
	err := pipeline.Run(input, run, output, 0)
	if err != nil {
		t.Fatal(err)
	}
	if results != times {
		t.Fatalf("output func ran %d times, not %d", results, times)
	}
}

func BenchmarkPipeline(b *testing.B) {
	input := func(add func(job) bool) {
		for i := 0; i < b.N; i++ {
			add(job(i))
		}
	}
	output := func(in job, out result, err error) error {
		return nil
	}
	b.ResetTimer()
	b.ReportAllocs()
	err := pipeline.Run(input, run, output, 0)
	if err != nil {
		b.Fatal(err)
	}
}
