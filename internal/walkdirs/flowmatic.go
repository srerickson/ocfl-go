package walkdirs

// Code in this file was adapted from Carl Johnson's "flowmatic" package,
// distibuted with the following license.

// MIT License

// Copyright (c) 2022 Carl Johnson

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

import (
	"runtime"
	"sync"

	"github.com/carlmjohnson/deque"
)

// Manager is a function that serially examines Task results to see if it produced any new Inputs.
// Returning false will halt the processing of future tasks.
type Manager[Input, Output any] func(Input, Output, error) (tasks []Input, ok bool)

// Task is a function that can concurrently transform an input into an output.
type Task[Input, Output any] func(in Input) (out Output, err error)

// // DoTasks does tasks using n concurrent workers (or GOMAXPROCS workers if n <
// 1) which produce output consumed by a serially run manager. The manager
// should return a slice of new task inputs based on prior task results, or
// return false to halt processing. If a task panics during execution, the panic
// will be caught and rethrown in the parent Goroutine.
//
// DoTailingTasks is the same as DoTasks except tasks in the task queue are
// evaluated in last in, first out order.
func DoTailingTasks[Input, Output any](n int, task Task[Input, Output], manager Manager[Input, Output], initial ...Input) {
	in, out := start(n, task)
	defer func() {
		close(in)
		// drain any waiting tasks
		for range out {
		}
	}()
	queue := deque.Of(initial...)
	inflight := 0
	for inflight > 0 || queue.Len() > 0 {
		inch := in
		item, ok := queue.Tail()
		if !ok {
			inch = nil
		}
		select {
		case inch <- item:
			inflight++
			queue.PopTail()
		case r := <-out:
			inflight--
			if r.Panic != nil {
				panic(r.Panic)
			}
			items, ok := manager(r.In, r.Out, r.Err)
			if !ok {
				return
			}
			queue.Append(items...)
		}
	}
}

// result is the type returned by the output channel of Start.
type result[Input, Output any] struct {
	In    Input
	Out   Output
	Err   error
	Panic any
}

// start n workers (or GOMAXPROCS workers if n < 1) which consume
// the in channel, execute task, and send the Result on the out channel.
// Callers should close the in channel to stop the workers from waiting for tasks.
// The out channel will be closed once the last result has been sent.
func start[Input, Output any](n int, task Task[Input, Output]) (in chan<- Input, out <-chan result[Input, Output]) {
	if n < 1 {
		n = runtime.GOMAXPROCS(0)
	}
	inch := make(chan Input)
	ouch := make(chan result[Input, Output], n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			defer func() {
				pval := recover()
				if pval == nil {
					return
				}
				ouch <- result[Input, Output]{Panic: pval}
			}()
			for inval := range inch {
				outval, err := task(inval)
				ouch <- result[Input, Output]{inval, outval, err, nil}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(ouch)
	}()
	return inch, ouch
}
