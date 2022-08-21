package checksum

import (
	"context"
	"errors"
	"io/fs"
	"strings"
)

// JobFunc is a function called for each complete job by Walk(). The funciton is
// called in the same go routine as the call to Walk(). If JobFunc() returns an
// error, Walk will close the pipe and JobFunc will not be called again
type JobFunc func(Job, error) error

// WalkErr combines the two kinds of errors that Walk() may need to report
// in one object
type WalkErr struct {
	WalkDirErr error // error returned from WalkDir
	JobFuncErr error // error returned from JobFunc
}

// Error implements error interface for WalkErr
func (we *WalkErr) Error() string {
	var m []string
	if we.WalkDirErr != nil {
		m = append(m, we.WalkDirErr.Error())
	}
	if we.JobFuncErr != nil {
		m = append(m, we.JobFuncErr.Error())
	}
	return strings.Join(m, `; `)
}

// SkipFile is an error returned by a WalkDirFunc to signal that the item in the
// path should not be added to the Pipe
var ErrSkipFile = errors.New(`skip file`)

// DefaultWalkDirFunc is the defult WalkDirFunc used by Walk. It only adds
// regular files to the Pipe.
func DefaultWalkDirFunc(path string, d fs.DirEntry, err error) error {
	if err != nil {
		return err
	}
	if !d.Type().IsRegular() {
		// don't checksum
		return ErrSkipFile
	}
	return nil
}

func Walk(fsys fs.FS, root string, each JobFunc, opts ...func(*Config)) error {
	conf := defaultConfig()
	for _, opt := range opts {
		opt(&conf)
	}
	// this cancel is used if each() returns an error for a job.
	// The cancel causes additional calls to Add() to retun
	// an error
	var cancel context.CancelFunc
	conf.ctx, cancel = context.WithCancel(conf.ctx)

	p, err := NewPipe(fsys, withConfig(&conf))
	if err != nil {
		cancel()
		return err
	}
	walkErrChan := make(chan error, 1)
	go func() {
		defer p.Close()
		defer close(walkErrChan)
		walk := func(path string, d fs.DirEntry, e error) error {
			if err := p.conf.walkDirFunc(path, d, e); err != nil {
				if err == ErrSkipFile {
					return nil // continue walk but no checksum
				}
				return err
			}
			return p.Add(path)
		}
		walkErrChan <- fs.WalkDir(fsys, root, walk)
	}()

	// process job callbacks and capture errors
	var jobFuncErr error
	for complete := range p.Out() {
		if jobFuncErr == nil {
			jobFuncErr = each(complete, complete.Err())
			if jobFuncErr != nil {
				cancel()
			}
		}
	}
	walkErr := <-walkErrChan
	if jobFuncErr != nil || walkErr != nil {
		cancel()
		return &WalkErr{
			WalkDirErr: walkErr,
			JobFuncErr: jobFuncErr,
		}
	}
	cancel()
	return nil
}
