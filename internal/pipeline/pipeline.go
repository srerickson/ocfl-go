package pipeline

import (
	"iter"
	"runtime"
	"sync"
)

// pipeline is a parameterized type for concurrent job processing
type pipeline[Tin, Tout any] struct {
	numWorkers int
	taskIter   iter.Seq[Tin]
	workFn     func(Tin) (Tout, error)
}

type Result[Tin, Tout any] struct {
	In  Tin
	Out Tout
	Err error
}

type ResultSeq[Tin, Tout any] func(func(Result[Tin, Tout]) bool)

// Results is a generic implementation of the fan-out/fan-in concurrency
// pattern. The input paramateter, tasks, is an iterator function for adding
// tasks of generic type type, Tin, to the work queue. Tasks are processed
// using taskFn in separate go routines. Use numWorkers to set the number of
// go routines for processing tasks. if numWorkers is < 1, it is set to the
// value from runtime.GOMAXPROCS(0). Results returns an iterattor that yields
// individual Result values. If the yield function of the returned iterator
// returns false, task processing is stopped and no new tasks are received.
// the yield function runs in the same go routine as the caller.
func Results[Tin, Tout any](tasks iter.Seq[Tin], workFn func(Tin) (Tout, error), numWorkers int) ResultSeq[Tin, Tout] {
	p := &pipeline[Tin, Tout]{
		numWorkers: numWorkers,
		taskIter:   tasks,
		workFn:     workFn,
	}
	return p.results
}

func (p *pipeline[Tin, Tout]) results(yield func(Result[Tin, Tout]) bool) {
	if p.workFn == nil {
		return
	}
	if p.taskIter == nil {
		return
	}
	if p.numWorkers < 1 {
		p.numWorkers = runtime.GOMAXPROCS(0)
	}
	taskQ := make(chan Tin)
	resultQ := make(chan Result[Tin, Tout], p.numWorkers)
	cancel := make(chan struct{})
	defer func() {
		// cancel context and drain result channel
		close(cancel)
		for range resultQ {
		}
	}()
	// iterate over tasks
	go func() {
		defer close(taskQ)
		for task := range p.taskIter {
			select {
			case taskQ <- task:
			case <-cancel:
				return
			}
		}
	}()
	// process tasks
	wg := sync.WaitGroup{}
	wg.Add(p.numWorkers)
	for i := 0; i < p.numWorkers; i++ {
		go func() {
			defer wg.Done()
			for in := range taskQ {
				select {
				case <-cancel:
				default:
					r := Result[Tin, Tout]{In: in}
					r.Out, r.Err = p.workFn(r.In)
					resultQ <- r
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(resultQ)
	}()
	// yield task results
	for r := range resultQ {
		if !yield(r) {
			return
		}
	}
}
