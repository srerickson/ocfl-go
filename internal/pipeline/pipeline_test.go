package pipeline_test

import (
	"errors"
	"testing"

	"github.com/srerickson/ocfl/internal/pipeline"
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

func TestPipelineSetupErr(t *testing.T) {
	input := func(add func(job) error) error {
		return errors.New("catch me")
	}
	output := func(in job, out result, err error) error {
		return err
	}
	err := pipeline.Run(input, run, output, 0)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestPipelineResultErr(t *testing.T) {
	input := func(add func(job) error) error {
		add(job(-1)) // invalid job
		return nil
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
	input := func(add func(job) error) error {
		for i := 0; i < times; i++ {
			add(job(i))
		}
		return nil
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
	input := func(add func(job) error) error {
		for i := 0; i < b.N; i++ {
			if err := add(job(i)); err != nil {
				return err
			}
		}
		return nil
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
