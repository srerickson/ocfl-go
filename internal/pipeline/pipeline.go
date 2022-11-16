package pipeline

import (
	"context"
	"runtime"
	"sync"
)

// pipeline is a parameterized type for processing generic Jobs
// concurrently
type pipeline[Tin, Tout any] struct {
	setupFn  func(func(Tin) error) error
	workFn   func(context.Context, Tin) (Tout, error)
	resultFn func(Tout) error
	numgos   int
	workQ    chan Tin
	resultQ  chan Tout
	workWG   sync.WaitGroup
	cancel   context.CancelFunc
	err      error
	errOnce  sync.Once
}

// Run is a generic implementation of the fan-out/fan-in concurrency pattern.
// The setup function is used to add values to a work queue; values are
// processed in a separate go routines using the work function; finally, result
// values are returned through the callback function, result, which runs in the
// same go routine used to call Run(). Use gos to set the maximum number of
// worker go routines (the default is runtime.NumCPU()). If setupFn, workFn, or
// resultFn returns an error, the internal context for Run is cancelled and
// the error is returned as Run's return value.
func Run[Tin, Tout any](
	ctx context.Context,
	setupFn func(func(Tin) error) error,
	workFn func(context.Context, Tin) (Tout, error),
	resultFn func(Tout) error, gos int) error {

	return (&pipeline[Tin, Tout]{
		numgos:   gos,
		setupFn:  setupFn,
		workFn:   workFn,
		resultFn: resultFn,
	}).run(ctx)
}

func (p *pipeline[Tin, Tout]) run(ctx context.Context) error {
	if p.numgos < 1 {
		p.numgos = runtime.NumCPU()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	p.cancel = cancel
	p.workQ = make(chan Tin, p.numgos)
	p.resultQ = make(chan Tout, p.numgos)

	// jobs in
	go func() {
		defer close(p.workQ)
		if p.setupFn == nil {
			return
		}
		addWork := func(w Tin) error {
			select {
			case p.workQ <- w:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		err := p.setupFn(addWork)
		if err != nil {
			p.setError(err)
		}
	}()

	// workers
	p.workWG.Add(p.numgos)
	for i := 0; i < p.numgos; i++ {
		go p.worker(ctx)
	}
	go func() {
		defer close(p.resultQ)
		p.workWG.Wait()
	}()

	// jobs out
	for j := range p.resultQ {
		if p.resultFn != nil {
			if err := p.resultFn(j); err != nil {
				p.setError(err)
			}
		}
	}
	return p.err
}

func (p *pipeline[Tin, Tout]) worker(ctx context.Context) {
	defer p.workWG.Done()
	for j := range p.workQ {
		var out Tout
		if p.workFn != nil {
			var err error
			out, err = p.workFn(ctx, j)
			if err != nil {
				p.setError(err)
				return
			}
		}
		select {
		case p.resultQ <- out:
		case <-ctx.Done():
			return
		}
	}
}

func (p *pipeline[Tin, Tout]) setError(err error) {
	if err != nil {
		if p.cancel != nil {
			p.cancel()
		}
		p.errOnce.Do(func() {
			p.err = err
		})
	}
}
