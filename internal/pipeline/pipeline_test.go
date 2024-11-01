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
	pipeline.Results[job, result](nil, nil, 0)(func(r pipeline.Result[job, result]) bool {
		t.Fatal("shouldn't run")
		return true
	})
}

func TestPipelineResultErr(t *testing.T) {
	input := func(add func(job) bool) {
		add(job(-1)) // invalid job
	}
	pipeline.Results(input, run, 4)(func(r pipeline.Result[job, result]) bool {
		if r.Err == nil {
			t.Fatal("expected an error")
		}
		return true
	})
}

func TestPipeline(t *testing.T) {
	times := 100
	input := func(add func(job) bool) {
		for i := 0; i < times; i++ {
			add(job(i))
		}
	}
	var results int
	pipeline.Results(input, run, 4)(func(r pipeline.Result[job, result]) bool {
		if r.Err != nil {
			t.Fatal(r.Err)
			return false
		}
		results++
		return true
	})
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
	b.ResetTimer()
	b.ReportAllocs()
	pipeline.Results(input, run, 0)(func(r pipeline.Result[job, result]) bool {
		return r.Err == nil
	})
}
