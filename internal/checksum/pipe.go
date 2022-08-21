package checksum

// Copyright 2020 Seth R. Erickson
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import (
	"errors"
	"io/fs"
	"sync"
)

// A Pipe performs concurrent checksum processing. It has an input channel and
// an output channel and a configurable number of go routines that process Jobs.
// Pipes are created with NewPipe(). Jobs are added to the Pipe with Add().
// Processed Jobs are added to the channel returned by Out(). Add() and Out()
// should be called from separate go routines to avoid deadlocks (See Walk() for
// an example).
//
// The Close() method must be called to properly free resource of Pipes created
// with NewPipe.
type Pipe struct {
	conf Config   // common config options
	fsys fs.FS    // the pipe's jobs are scoped to the fs
	in   chan Job // jop input
	out  chan Job // job results
}

// NewPipe returns a new Pipe scoped to fsys. The following functional options
// are use to configre the Pipe (with defaults):
//  - With[Alg](): Required
//  - WithCtx(): context.Background().
//  - WithNumGos():runtime.GOMAXPROCS(0)
func NewPipe(fsys fs.FS, opts ...func(*Config)) (*Pipe, error) {
	pipe := &Pipe{
		fsys: fsys,
		in:   make(chan Job),
		out:  make(chan Job),
		conf: defaultConfig(),
	}
	for _, option := range opts {
		option(&pipe.conf)
	}

	var wg sync.WaitGroup
	for i := 0; i < pipe.conf.numGos; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range pipe.in {
				select {
				case <-pipe.conf.ctx.Done():
					continue // clear input channel
				default:
					job.do()
					pipe.out <- job
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(pipe.out)
	}()
	return pipe, nil
}

// Out returns the Pipe's recieve-only channel of Job results
func (p *Pipe) Out() <-chan Job {
	return p.out
}

// Close frees the resources used by the Pipe. Calling Add() after Close() will
// cause a panic.
func (p *Pipe) Close() {
	close(p.in)
}

// Add adds a checksum job for path to the Pipe. The path is evaluated in the
// context of the Pipe's fs.FS. Optional arguments may be used to specify the
// hash algorithms the job should use (these options supercede options to
// NewPipe). It returns an error if the Pipe context is canceled or if no hash
// algorithms are defined for the Pipe or the Job. In both cases, the job is
// not created. It causes a panic if called after Close(). To avoid deadlocks,
// Add should be called in a separate go routine than Out().
func (p *Pipe) Add(path string, opts ...func(*Config)) error {
	var conf Config
	jobAlgs := p.conf.algs
	for _, option := range opts {
		option(&conf)
	}
	if conf.algs != nil {
		jobAlgs = conf.algs
	}
	if jobAlgs == nil {
		return errors.New(`checksum aglorithm not set`)
	}
	select {
	case <-p.conf.ctx.Done():
		return p.conf.ctx.Err()
	default:
		p.in <- Job{
			path: path,
			fs:   p.fsys,
			algs: jobAlgs,
		}
	}
	return nil

}
