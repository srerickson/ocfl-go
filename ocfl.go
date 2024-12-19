// This module is an implementation of the Oxford Common File Layout (OCFL)
// specification. The top-level package provides version-independent
// functionality. The ocflv1 package provides the bulk of implementation.
package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
)

const (
	// package version
	Version = "0.6.0"

	Spec1_0 = Spec("1.0")
	Spec1_1 = Spec("1.1")

	logsDir       = "logs"
	contentDir    = "content"
	extensionsDir = "extensions"
	inventoryBase = "inventory.json"
)

var (
	OCFLv1_0 ocflImp = &ocflV1{spec: Spec1_0}
	OCFLv1_1 ocflImp = &ocflV1{spec: Spec1_1}

	ErrOCFLNotImplemented    = errors.New("no implementation for the given OCFL specification version")
	ErrObjectNamasteExists   = fmt.Errorf("found existing OCFL object declaration: %w", fs.ErrExist)
	ErrObjectNamasteNotExist = fmt.Errorf("the OCFL object declaration does not exist: %w", ErrNamasteNotExist)
	ErrObjRootStructure      = errors.New("object includes invalid files or directories")
)

// ocflImp is an interface implemented by types that implement a specific
// version of the ocflImp specification.
type ocflImp interface {
	Spec() Spec
	NewInventory(raw []byte) (Inventory, error)
	Commit(ctx context.Context, obj *Object, commit *Commit) error
	// validate an existing Inventory
	ValidateInventory(Inventory) *Validation
	// fully validate raw inventory bytes, returning it if there are no fatal errors
	ValidateInventoryBytes([]byte) (Inventory, *Validation)
	ValidateObjectRoot(ctx context.Context, v *ObjectValidation, state *ObjectState) error
	ValidateObjectVersion(ctx context.Context, v *ObjectValidation, vnum VNum, versionInv, prevInv Inventory) error
	ValidateObjectContent(ctx context.Context, v *ObjectValidation) error
}

// getOCFL is returns the implemenation for a given version of the OCFL spec.
func getOCFL(spec Spec) (ocflImp, error) {
	switch spec {
	case Spec1_0, Spec1_1:
		return &ocflV1{spec: spec}, nil
	}
	return nil, fmt.Errorf("%w: v%s", ErrOCFLNotImplemented, spec)
}

// returns the earliest OCFL implementation (OCFL v1.0)
func lowestOCFL() ocflImp { return &ocflV1{Spec1_0} }

// returns the latest OCFL implementation (OCFL v1.1)
func latestOCFL() ocflImp { return &ocflV1{Spec1_1} }

// mustGetOCFL is like getOCFL except it panics if the implemenation is not
// found.
func mustGetOCFL(spec Spec) ocflImp {
	impl, err := getOCFL(spec)
	if err != nil {
		panic(err)
	}
	return impl
}

// defaultOCFL returns the default OCFL implementation (v1.1).
func defaultOCFL() ocflImp { return latestOCFL() }
