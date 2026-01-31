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
	Version = "0.11.1"

	Spec1_0 = Spec("1.0")
	Spec1_1 = Spec("1.1")

	logsDir       = "logs"
	contentDir    = "content"
	extensionsDir = "extensions"
	inventoryBase = "inventory.json"
)

var (
	ErrOCFLNotImplemented    = errors.New("unimplemented or missing version of the OCFL specification")
	ErrObjectNamasteExists   = fmt.Errorf("found existing OCFL object declaration: %w", fs.ErrExist)
	ErrObjectNamasteNotExist = fmt.Errorf("the OCFL object declaration does not exist: %w", ErrNamasteNotExist)
	ErrObjRootStructure      = errors.New("object includes invalid files or directories")
)

// ocfl is an interface implemented by types that implement a specific
// version of the ocfl specification.
type ocfl interface {
	// Spec returns the implemented version of the OCFL specification
	Spec() Spec
	// ValidateInventory validates an existing Inventory value.
	ValidateInventory(*Inventory) *Validation
	// ValidateInventoryBytes fully validates bytes as a json-encoded inventory.
	// It returns the Inventory if the validation result does not included fatal
	// errors.
	ValidateInventoryBytes([]byte) (*StoredInventory, *Validation)
	// Validate all contents of an object root: NAMASTE, inventory, sidecar, etc.
	ValidateObjectRoot(ctx context.Context, v *ObjectValidation, state *ObjectState) error
	// Validate all contents of an object version directory and add contents to the object validation
	ValidateObjectVersion(ctx context.Context, v *ObjectValidation, vnum VNum, versionInv, prevInv *StoredInventory) error
	// Validate contents added to the object validation.
	ValidateObjectContent(ctx context.Context, v *ObjectValidation) error
}

// getOCFL is returns the implemenation for a given version of the OCFL spec.
func getOCFL(spec Spec) (ocfl, error) {
	switch spec {
	case Spec1_0, Spec1_1:
		return &ocflV1{v1Spec: spec}, nil
	case Spec(""):
		return nil, ErrOCFLNotImplemented
	}
	return nil, fmt.Errorf("%w: v%s", ErrOCFLNotImplemented, spec)
}

// returns the earliest OCFL implementation (OCFL v1.0)
func lowestOCFL() ocfl { return &ocflV1{Spec1_0} }

// returns the latest OCFL implementation (OCFL v1.1)
func latestOCFL() ocfl { return &ocflV1{Spec1_1} }

// mustGetOCFL is like getOCFL except it panics if the implemenation is not
// found.
func mustGetOCFL(spec Spec) ocfl {
	impl, err := getOCFL(spec)
	if err != nil {
		panic(err)
	}
	return impl
}

// defaultOCFL returns the default OCFL implementation (v1.1).
func defaultOCFL() ocfl { return latestOCFL() }
