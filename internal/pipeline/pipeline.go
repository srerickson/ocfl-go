package pipeline

import (
	"runtime"
	"sync"
)

// pipeline is a parameterized type for processing generic Jobs
// concurrently
type pipeline[Tin, Tout any] struct {
	numgos   int
	setupFn  func(func(Tin) bool)
	workFn   func(Tin) (Tout, error)
	resultFn func(Tin, Tout, error) error
}

type result[Tin, Tout any] struct {
	in  Tin
	out Tout
	err error
}

// Run is a generic implementation of the fan-out/fan-in concurrency pattern.
// The setup function is used to add values to a work queue; values are
// processed in separate go routines using the work function; finally, result
// values are returned through the callback function, result, which runs in the
// same go routine used to call Run(). Use gos to set the maximum number of
// worker go routines (the default is runtime.NumCPU()).

func Run[Tin, Tout any](
	setupFn func(func(Tin) bool),
	workFn func(Tin) (Tout, error),
	resultFn func(Tin, Tout, error) error, gos int) error {

	return (&pipeline[Tin, Tout]{
		numgos:   gos,
		setupFn:  setupFn,
		workFn:   workFn,
		resultFn: resultFn,
	}).run()
}

func (p *pipeline[Tin, Tout]) run() error {
	if p.numgos < 1 {
		p.numgos = runtime.NumCPU()
	}
	// job input queue
	workQ := make(chan Tin)
	// close to prevent new jobs in workQ
	workQcancel := make(chan struct{})
	// completed work
	resultQ := make(chan result[Tin, Tout], p.numgos)
	go func() {
		defer close(workQ)
		if p.setupFn == nil {
			return
		}
		// addWork is the function passed back to setupFn for adding jobs to the
		// workQ
		addWork := func(w Tin) bool {
			select {
			case workQ <- w:
				return true
			case <-workQcancel:
				return false
			}
		}
		p.setupFn(addWork)
	}()
	// workers
	wg := sync.WaitGroup{}
	wg.Add(p.numgos)
	for i := 0; i < p.numgos; i++ {
		go func() {
			defer wg.Done()
			for in := range workQ {
				r := result[Tin, Tout]{in: in}
				if p.workFn != nil {
					r.out, r.err = p.workFn(r.in)
				}
				resultQ <- r
			}
		}()
	}
	go func() {
		wg.Wait()
		close(resultQ)
	}()

	// jobs out
	var resultErr error
	for r := range resultQ {
		if p.resultFn == nil {
			continue
		}
		if resultErr != nil {
			// if resultFn returns an error, it's not called again.
			// continue to drain resultQ
			continue
		}
		if err := p.resultFn(r.in, r.out, r.err); err != nil {
			resultErr = err
			close(workQcancel)
		}
	}
	if resultErr == nil {
		// term was only closed if err from resultQ
		close(workQcancel)
	}
	return resultErr
}
