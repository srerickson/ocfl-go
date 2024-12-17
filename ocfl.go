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
	inventoryFile = "inventory.json"
	contentDir    = "content"
	extensionsDir = "extensions"
)

var (
	OCFLv1_0 implemenation = &ocflV1{spec: Spec1_0}
	OCFLv1_1 implemenation = &ocflV1{spec: Spec1_1}

	ErrOCFLNotImplemented    = errors.New("no implementation for the given OCFL specification version")
	ErrObjectNamasteExists   = fmt.Errorf("found existing OCFL object declaration: %w", fs.ErrExist)
	ErrObjectNamasteNotExist = fmt.Errorf("the OCFL object declaration does not exist: %w", ErrNamasteNotExist)
	ErrObjRootStructure      = errors.New("object includes invalid files or directories")
)

// implemenation is an interface implemented by types that implement a specific
// version of the implemenation specification.
type implemenation interface {
	Spec() Spec
	NewInventory(raw []byte) (Inventory, error)
	NewReadObject(fsys FS, path string, inv Inventory) ReadObject
	Commit(ctx context.Context, obj ReadObject, commit *Commit) (ReadObject, error)
	ValidateInventoryBytes([]byte) (Inventory, *Validation)
	ValidateObjectRoot(ctx context.Context, fs FS, dir string, state *ObjectState, vldr *ObjectValidation) (ReadObject, error)
	ValidateObjectVersion(ctx context.Context, obj ReadObject, vnum VNum, versionInv Inventory, prevInv Inventory, vldr *ObjectValidation) error
	ValidateObjectContent(ctx context.Context, obj ReadObject, vldr *ObjectValidation) error
}

// config is includes shared configuration for objects and storage roots.
// TODO: available extensions.
type config struct{}

// getOCFL is returns the implemenation for a given version of the OCFL spec.
func (c config) getOCFL(spec Spec) (implemenation, error) {
	switch spec {
	case Spec1_0:
		return OCFLv1_0, nil
	case Spec1_1:
		return OCFLv1_1, nil
	default:
		return nil, fmt.Errorf("%w: v%s", ErrOCFLNotImplemented, spec)
	}
}

// mustGetOCFL is like getOCFL except it panics if the implemenation is not
// found.
func (c config) mustGetOCFL(spec Spec) implemenation {
	impl, err := c.getOCFL(spec)
	if err != nil {
		panic(err)
	}
	return impl
}

// defaultOCFL returns the default OCFL implementation (v1.1).
func (c config) defaultOCFL() implemenation { return OCFLv1_1 }

type ReadObject interface {
	// Inventory returns the object's inventory or nil if
	// the object hasn't been created yet.
	Inventory() Inventory
	// FS for accessing object contents
	FS() FS
	// Path returns the object's path relative to its FS()
	Path() string
	// VersionFS returns an io/fs.FS for accessing the logical contents of the
	// object version state with the index v.
	VersionFS(ctx context.Context, v int) fs.FS
}
