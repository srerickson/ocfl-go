package validation

import "sync"

type Result struct {
	lock  sync.RWMutex
	fatal []error
	warn  []error
}

func (r *Result) AddFatal(err error) error {
	if err == nil {
		return nil
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	r.fatal = append(r.fatal, err)
	return err
}

func (r *Result) AddWarn(err error) error {
	if err == nil {
		return nil
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	r.warn = append(r.warn, err)
	return err
}

func (r *Result) Valid() bool {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return len(r.fatal) == 0
}

func (r *Result) Fatal() []error {
	r.lock.RLock()
	defer r.lock.RUnlock()
	var f []error
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

func (r *Result) Warn() []error {
	r.lock.RLock()
	defer r.lock.RUnlock()
	var f []error
	return append(f, r.warn...)
}

func (r *Result) Merge(re *Result) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.fatal = append(r.fatal, re.fatal...)
	r.warn = append(r.warn, re.warn...)
}
