package validation

import (
	"sync"

	"github.com/go-logr/logr"
)

const (
	fatal          = "fatal"
	warn           = "warning"
	defaultMaxErrs = 100 // default
)

type Result struct {
	// maxErrs is the capacity of the fatal and warn errors (each) in the
	// result. If not set, maxErrs will be set to 100. Set to -1 for no limit.
	maxErrs int

	fatal []error
	warn  []error
	lock  sync.RWMutex
}

// NewResult creates a new Result that can hold up to max fatal and warning
// errors (each). If max is -1, the Result has no limit.
func NewResult(max int) *Result {
	if max == 0 {
		max = defaultMaxErrs
	}
	cap := max // capacity for initial slices
	if cap > 8 || cap < 0 {
		cap = 8
	}
	return &Result{
		maxErrs: max,
		fatal:   make([]error, 0, cap),
		warn:    make([]error, 0, cap),
	}
}

// AddFatal adds err to the Result as a fatal error
func (r *Result) AddFatal(err error) *Result {
	if err == nil {
		return r
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.maxErrs == 0 {
		r.maxErrs = defaultMaxErrs
	}
	if len(r.fatal) >= r.maxErrs && r.maxErrs > 0 {
		return r
	}
	r.fatal = append(r.fatal, err)
	return r
}

// AddWarn adds err to the Result as a warning error
func (r *Result) AddWarn(err error) *Result {
	if err == nil {
		return r
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.maxErrs == 0 {
		r.maxErrs = defaultMaxErrs
	}
	if len(r.warn) >= r.maxErrs && r.maxErrs > 0 {
		return r
	}
	r.warn = append(r.warn, err)
	return r
}

// LogFatal adds err to the result as a fatal error and writes err to the logger
func (r *Result) LogFatal(logger logr.Logger, err error) *Result {
	if err != nil {
		r.AddFatal(err)
		r.logType(logger, err, fatal)
	}
	return r
}

// LogWarn adds err to the result as a warning error and writes err to the
// logger
func (r *Result) LogWarn(logger logr.Logger, err error) *Result {
	if err != nil {
		r.AddWarn(err)
		r.logType(logger, err, warn)
	}
	return r
}

func (r *Result) Valid() bool {
	return r.Err() == nil
}

// Fatal returns a slice of all the fatal errors in r
func (r *Result) Fatal() []error {
	r.lock.RLock()
	defer r.lock.RUnlock()
	f := make([]error, 0, len(r.fatal))
	return append(f, r.fatal...)
}

// Err returns the last fatal err
func (r *Result) Err() error {
	r.lock.RLock()
	defer r.lock.RUnlock()
	if len(r.fatal) == 0 {
		return nil
	}
	return r.fatal[len(r.fatal)-1]
}

// Warn returns a slice of all the warning errors in r
func (r *Result) Warn() []error {
	r.lock.RLock()
	defer r.lock.RUnlock()
	f := make([]error, 0, len(r.warn))
	return append(f, r.warn...)
}

// Merge adds all errors in src to r, up the limit set
// by r.MaxErrs.
func (r *Result) Merge(src *Result) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.maxErrs == 0 {
		r.maxErrs = defaultMaxErrs
	}
	s := len(r.fatal)
	newS := s + len(src.fatal)
	if newS <= r.maxErrs || r.maxErrs < 0 {
		r.fatal = append(r.fatal, src.fatal...)
	} else if s < r.maxErrs {
		r.fatal = append(r.fatal, src.fatal[:r.maxErrs-s]...)
	}
	s = len(r.warn)
	newS = s + len(src.warn)
	if newS <= r.maxErrs || r.maxErrs < 0 {
		r.warn = append(r.warn, src.warn...)
	} else if s < r.maxErrs {
		r.warn = append(r.warn, src.warn[:r.maxErrs-s]...)
	}
}

// Log logs all errs in the in reset to the logger
func (r *Result) Log(logger logr.Logger) {
	for _, err := range r.warn {
		r.logType(logger, err, warn)
	}
	for _, err := range r.fatal {
		r.logType(logger, err, fatal)
	}
}

func (r *Result) logType(logger logr.Logger, err error, typ string) {
	vals := []interface{}{"type", typ}
	if vErr, ok := err.(ErrorCode); ok {
		if ref := vErr.OCFLRef(); ref != nil {
			vals = append(vals, "OCFL", ref.Code)
		}
	}
	logger.Info(err.Error(), vals...)
}
