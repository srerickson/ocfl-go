// This module is an implementation of the Oxford Common File Layout (OCFL)
// specification. The top-level package provides version-independent
// functionality. The ocflv1 package provides the bulk of implementation.
package ocfl

import (
	"context"
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
	digestConcurrency atomic.Int32 // FIXME: get rid of this
	commitConcurrency atomic.Int32 // FIXME: get rid of this

	ocflRegister   = map[Spec]OCFL{}
	ocflRegisterMx sync.Mutex
)

// OCFL is an interface implemented by types that implement a specific
// version of the OCFL specification.
type OCFL interface {
	Spec() Spec
	NewObject(context.Context, *ObjectRoot, ...func(*ObjectOptions)) (Object, error)
	// SorageRoot
	// Validate
}

func RegisterOCLF(imp OCFL) bool {
	ocflRegisterMx.Lock()
	defer ocflRegisterMx.Unlock()
	if _, exists := ocflRegister[imp.Spec()]; exists {
		return false
	}
	ocflRegister[imp.Spec()] = imp
	return true
}

func GetOCFL(spec Spec) OCFL {
	ocflRegisterMx.Lock()
	defer ocflRegisterMx.Unlock()
	return ocflRegister[spec]
}

// UnsetOCFL removes the previously set implementation for spec, if
// present. It returns true if the implementation was removed and false if no
// implementation was found for the spec.
func UnsetOCFL(spec Spec) bool {
	ocflRegisterMx.Lock()
	defer ocflRegisterMx.Unlock()
	if _, exists := ocflRegister[spec]; !exists {
		return false
	}
	delete(ocflRegister, spec)
	return true
}

func Implementations() []Spec {
	ocflRegisterMx.Lock()
	defer ocflRegisterMx.Unlock()
	specs := make([]Spec, 0, len(ocflRegister))
	for spec := range ocflRegister {
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
