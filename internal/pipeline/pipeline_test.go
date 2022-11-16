package pipeline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/srerickson/ocfl/internal/pipeline"
)

type job struct {
	err error
}

type result struct {
	err error
}

func run(ctx context.Context, j job) (result, error) {
	return result(j), j.err
}

func TestPipelineNil(t *testing.T) {
	err := pipeline.Run[job, result](context.Background(), nil, nil, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPipelineErr(t *testing.T) {
	ctx := context.Background()
	input := func(add func(job) error) error {
		for i := 0; i < 5; i++ {
			if err := add(job{}); err != nil {
				return err
			}
		}
		add(job{errors.New("catch me")})
		for i := 0; i < 5; i++ {
			if err := add(job{}); err != nil {
				return err
			}
		}
		return nil
	}
	output := func(r result) error {
		return r.err
	}
	err := pipeline.Run(ctx, input, run, output, 0)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestPipeline(t *testing.T) {
	times := 100
	ctx := context.Background()
	input := func(add func(job) error) error {
		for i := 0; i < times; i++ {
			if err := add(job{}); err != nil {
				return err
			}
		}
		return nil
	}
	var results int
	output := func(r result) error {
		results++
		return nil
	}
	err := pipeline.Run(ctx, input, run, output, 0)
	if err != nil {
		t.Fatal(err)
	}
	if results != times {
		t.Fatalf("output func ran %d times, not %d", results, times)
	}
}

func BenchmarkPipeline(b *testing.B) {
	ctx := context.Background()
	input := func(add func(job) error) error {
		for i := 0; i < b.N; i++ {
			if err := add(job{}); err != nil {
				return err
			}
		}
		return nil
	}
	output := func(r result) error {
		return nil
	}
	b.ResetTimer()
	b.ReportAllocs()
	err := pipeline.Run(ctx, input, run, output, 0)
	if err != nil {
		b.Fatal(err)
	}
}
