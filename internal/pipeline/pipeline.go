package pipeline

import (
	"errors"
	"runtime"
	"sync"
)

var ErrNotAdded = errors.New("task not added to pipeline")

// pipeline is a parameterized type for processing generic Jobs
// concurrently
type pipeline[Tin, Tout any] struct {
	numgos   int
	setupFn  func(func(Tin) error) error
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
	setupFn func(func(Tin) error) error,
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
	workQ := make(chan Tin)
	resultQ := make(chan result[Tin, Tout], p.numgos)
	term := make(chan struct{})
	// jobs in
	setupErr := make(chan error, 1)
	go func() {
		defer close(workQ)
		defer close(setupErr)
		if p.setupFn == nil {
			setupErr <- nil
			return
		}
		addWork := func(w Tin) error {
			select {
			case workQ <- w:
				return nil
			case <-term:
				return ErrNotAdded
			}
		}
		setupErr <- p.setupFn(addWork)
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
		// if resultFn returns an error, it's not called again.
		if err := p.resultFn(r.in, r.out, r.err); err != nil {
			resultErr = err
			break
		}
	}
	// at this point channels have either closed normally
	// or they should be closed because of an error from
	// resultFn. Prevent new tasks from setup.
	close(term)
	return errors.Join(resultErr, <-setupErr)
}
