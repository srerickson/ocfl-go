// This module is an implementation of the Oxford Common File Layout (OCFL)
// specification. The top-level package provides version-independent
// functionality. The ocflv1 package provides the bulk of implementation.
package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"runtime"
	"sync"
	"sync/atomic"
)

const (
	// package version
	Version       = "0.0.25"
	ExtensionsDir = "extensions"
)

var (
	ErrOCFLNotImplemented    = errors.New("no implementation for the given OCFL specification version")
	ErrObjectNamasteExists   = fmt.Errorf("found existing OCFL object declaration: %w", fs.ErrExist)
	ErrObjectNamasteNotExist = fmt.Errorf("the OCFL object declaration does not exist: %w", ErrNamasteNotExist)

	digestConcurrency atomic.Int32 // FIXME: get rid of this
	commitConcurrency atomic.Int32 // FIXME: get rid of this

	// map of OCFL implementations
	defaultOCFLs OCLFRegister
)

// OCFL is an interface implemented by types that implement a specific
// version of the OCFL specification.
type OCFL interface {
	Spec() Spec
	OpenObject(context.Context, *ObjectRoot, ...func(*ObjectOptions)) (Object, error)
	// SorageRoot
	// Validate
}

type Config struct {
	OCFLs *OCLFRegister
}

type OCLFRegister struct {
	ocfls   map[Spec]OCFL
	ocflsMx sync.RWMutex
	latest  OCFL
}

func GetOCFL(spec Spec) (OCFL, error) { return defaultOCFLs.Get(spec) }
func RegisterOCLF(imp OCFL) bool      { return defaultOCFLs.Set(imp) }
func UnsetOCFL(spec Spec) bool        { return defaultOCFLs.Unset(spec) }
func LatestOCFL() (OCFL, error)       { return defaultOCFLs.Latest() }
func Implementations() []Spec         { return defaultOCFLs.Specs() }

func (reg *OCLFRegister) Get(spec Spec) (OCFL, error) {
	reg.ocflsMx.RLock()
	defer reg.ocflsMx.RUnlock()
	if imp := reg.ocfls[spec]; imp != nil {
		return imp, nil
	}
	return nil, ErrOCFLNotImplemented
}

func (ocfl *OCLFRegister) Set(imp OCFL) bool {
	newSpec := imp.Spec()
	if err := newSpec.Valid(); err != nil {
		return false
	}
	ocfl.ocflsMx.Lock()
	defer ocfl.ocflsMx.Unlock()
	if _, exists := ocfl.ocfls[newSpec]; exists {
		return false
	}
	if ocfl.ocfls == nil {
		ocfl.ocfls = map[Spec]OCFL{}
	}
	ocfl.ocfls[newSpec] = imp
	if ocfl.latest == nil || newSpec.Cmp(ocfl.latest.Spec()) > 0 {
		ocfl.latest = imp
	}
	return true
}

// UnsetOCFL removes the previously set implementation for spec, if
// present. It returns true if the implementation was removed and false if no
// implementation was found for the spec.
func (reg *OCLFRegister) Unset(spec Spec) bool {
	reg.ocflsMx.Lock()
	defer reg.ocflsMx.Unlock()
	if _, exists := reg.ocfls[spec]; !exists {
		return false
	}
	delete(reg.ocfls, spec)
	return true
}

func (reg *OCLFRegister) Latest() (OCFL, error) {
	reg.ocflsMx.RLock()
	defer reg.ocflsMx.RUnlock()
	if reg.latest == nil {
		return nil, ErrOCFLNotImplemented
	}
	return reg.latest, nil
}

func (reg *OCLFRegister) Specs() []Spec {
	reg.ocflsMx.RLock()
	defer reg.ocflsMx.RUnlock()
	specs := make([]Spec, 0, len(reg.ocfls))
	for spec := range reg.ocfls {
		specs = append(specs, spec)
	}
	return specs
}

// DigestConcurrency is a global configuration for the number  of files to
// digest concurrently.
func DigestConcurrency() int {
	i := digestConcurrency.Load()
	if i < 1 {
		return runtime.NumCPU()
	}
	return int(i)
}

// SetDigestConcurrency sets the max number of files to digest concurrently.
func SetDigestConcurrency(i int) {
	digestConcurrency.Store(int32(i))
}

// XferConcurrency is a global configuration for the maximum number of files
// transferred concurrently during a commit operation. It defaults to
// runtime.NumCPU().
func XferConcurrency() int {
	i := commitConcurrency.Load()
	if i < 1 {
		return runtime.NumCPU()
	}
	return int(i)
}

// SetXferConcurrency sets the maximum number of files transferred concurrently
// during a commit operation.
func SetXferConcurrency(i int) {
	commitConcurrency.Store(int32(i))
}
