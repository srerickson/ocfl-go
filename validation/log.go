package validation

import (
	"errors"

	"github.com/go-logr/logr"
)

type Log struct {
	*Result
	Logger logr.Logger
}

func NewLog(l logr.Logger) Log {
	return Log{
		Logger: l,
		Result: &Result{
			fatal: []error{},
			warn:  []error{},
		},
	}
}

// WithValues returns a new Log that includes key/values in all messages.
func (l Log) WithValues(keysVals ...any) Log {
	return Log{
		Result: l.Result,
		Logger: l.Logger.WithValues(keysVals...),
	}
}

func (l Log) WithName(name string) Log {
	return Log{
		Result: l.Result,
		Logger: l.Logger.WithName(name),
	}
}

func (l *Log) logWarning(err error) {
	if l.Logger.GetSink() != nil {
		vals := []interface{}{"type", "fatal"}
		var verr *vErr
		if errors.As(err, &verr) && verr.Code() != "" {
			vals = append(vals, "OCFL", verr.Code())
		}
		l.Logger.Info(err.Error(), vals...)
	}
}

func (l *Log) logFatal(err error) {
	if l.Logger.GetSink() != nil {
		vals := []interface{}{"type", "fatal"}
		var verr *vErr
		if errors.As(err, &verr) && verr.Code() != "" {
			vals = append(vals, "OCFL", verr.Code())
		}
		l.Logger.Info(err.Error(), vals...)
	}
}

func (l *Log) AddFatal(err error) error {
	if err == nil {
		return nil
	}
	if l.Result == nil {
		l.Result = &Result{}
	}
	l.logFatal(err)
	return l.Result.AddFatal(err)
}

func (l *Log) AddWarn(err error) {
	if err == nil {
		return
	}
	if l.Result == nil {
		l.Result = &Result{}
	}
	l.logWarning(err)
	l.Result.AddWarn(err)
}

func (l *Log) AddResult(r *Result) {
	if r == nil {
		return
	}
	for _, e := range r.fatal {
		l.logFatal(e)
	}
	for _, e := range r.warn {
		l.logWarning(e)
	}
	if l.Result == nil {
		l.Result = r
		return
	}
	l.Result.Merge(r)
}

func (l Log) Err() error {
	if l.Result == nil {
		return nil
	}
	return l.Result.Err()
}

func (l Log) Fatal() []error {
	if l.Result == nil {
		return nil
	}
	return l.Result.Fatal()
}

func (l Log) Warn() []error {
	if l.Result == nil {
		return nil
	}
	return l.Result.Warn()
}
