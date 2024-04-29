package pipeline

import (
	"sync"
)

// pipeline is a parameterized type for processing generic Jobs
// concurrently
type pipeline[Tin, Tout any] struct {
	workers   int
	inputIter func(func(Tin) bool)
	workFn    func(Tin) (Tout, error)
}

type Result[Tin, Tout any] struct {
	In  Tin
	Out Tout
	Err error
}

// Run is a generic implementation of the fan-out/fan-in concurrency pattern.
// The setup function is used to add values to a work queue; values are
// processed in separate go routines using the work function; finally, result
// values are returned through the callback function, result, which runs in the
// same go routine used to call Run(). Use gos to set the maximum number of
// worker go routines (the default is runtime.NumCPU()).
func Run[Tin, Tout any](
	inputIter func(func(Tin) bool),
	workerFn func(Tin) (Tout, error),
	numWorkers int,
) func(yield func(Result[Tin, Tout]) bool) {
	pipe := &pipeline[Tin, Tout]{
		workers:   numWorkers,
		inputIter: inputIter,
		workFn:    workerFn,
	}
	return pipe.resultIter
}

func (p *pipeline[Tin, Tout]) resultIter(yield func(Result[Tin, Tout]) bool) {
	if p.workers < 1 {
		p.workers = 1
	}
	workQ := make(chan Tin)
	resultQ := make(chan Result[Tin, Tout], p.workers)
	cancel := make(chan struct{})

	defer func() {
		// cancel context and drain result channel
		close(cancel)
		for range resultQ {
		}
	}()

	go func() {
		defer close(workQ)
		if p.inputIter == nil {
			return
		}
		p.inputIter(func(w Tin) bool {
			select {
			case workQ <- w:
				return true
			case <-cancel:
				return false
			}
		})
	}()
	// workers
	wg := sync.WaitGroup{}
	wg.Add(p.workers)
	for i := 0; i < p.workers; i++ {
		go func() {
			defer wg.Done()
			for in := range workQ {
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
	for r := range resultQ {
		if !yield(r) {
			return
		}
	}
}
