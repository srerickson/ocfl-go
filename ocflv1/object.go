package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/logging"
	"github.com/srerickson/ocfl/validation"
)

var (
	ErrNotObject          = errors.New("not an OCFL object")
	ErrOCFLVersion        = errors.New("unsupported OCFL version")
	ErrInventoryOpen      = errors.New("could not read inventory file")
	ErrInvSidecarOpen     = errors.New("could not read inventory sidecar file")
	ErrInvSidecarContents = errors.New("invalid inventory sidecar contents")
	ErrInvSidecarChecksum = errors.New("inventory digest doesn't match expected value from sidecar file")
	ErrDigestAlg          = errors.New("invalid digest algorithm")
	ErrObjRootStructure   = errors.New("object includes invalid files or directories")
)

// Object represents an existing OCFL v1.x object. Use GetObject() to initialize
// new Objects.
type Object struct {
	ocfl.ObjectRoot
	Inventory Inventory
}

// GetObject returns the Object at the path in fsys. It returns an error if the
// object's root directory doesn't include an object declaration file, or if the
// root inventory is invalid.
func GetObject(ctx context.Context, fsys ocfl.FS, dir string) (*Object, error) {
	root, err := ocfl.GetObjectRoot(ctx, fsys, dir)
	if err != nil {
		return nil, err
	}
	if !ocflVerSupported[root.Spec] {
		return nil, fmt.Errorf("%s: %w", root.Spec, ErrOCFLVersion)
	}
	if !root.HasInventory() {
		// what is the best error to use here?
		return nil, ErrInventoryOpen
	}
	obj := &Object{ObjectRoot: *root}
	if err := obj.SyncInventory(ctx); err != nil {
		return nil, err
	}
	return obj, nil
}

// State initializes a new ocfl.ObjectState for the object version with the
// given index. If the index is 0, the most recent version (HEAD) is used.
func (obj Object) State(i int) (*ocfl.ObjectState, error) {
	state, err := obj.Inventory.objectState(i)
	if err != nil {
		return nil, err
	}
	state.FS = obj.FS
	state.Root = obj.Path
	return state, nil
}

// SyncInventory downloads and validates the object's root inventory. If
// successful the object's Inventory value is updated.
func (obj *Object) SyncInventory(ctx context.Context) error {
	name := path.Join(obj.Path, inventoryFile)
	alg, err := digest.Get(obj.Algorithm)
	if err != nil {
		return fmt.Errorf("reading inventory: %w", err)
	}
	nolog := ValidationLogger(logging.DisabledLogger())
	inv, results := ValidateInventory(ctx, obj.FS, name, alg, nolog)
	if err := results.Err(); err != nil {
		return fmt.Errorf("reading inventory: %w", err)
	}
	obj.Inventory = *inv
	return nil
}

// Validate fully validates the Object. If the object is valid, the Object's inventory
// is updated with the inventory downloaded during validation.
func (obj *Object) Validate(ctx context.Context, opts ...ValidationOption) *validation.Result {
	newObj, r := ValidateObject(ctx, obj.FS, obj.Path, opts...)
	if r.Err() == nil {
		obj.Inventory = newObj.Inventory
	}
	return r
}

// Objects iterates over over the OCFL Object in fsys with the given path
// selector and calls fn for each. If an error is encountered while loading the
// object, the error is passed to fn. If fn returns an error the iteration
// process terminates.
func Objects(ctx context.Context, fsys ocfl.FS, pth ocfl.PathSelector, fn func(*Object, error) error) error {
	eachRoot := func(root *ocfl.ObjectRoot) error {
		obj := Object{ObjectRoot: *root}
		return fn(&obj, obj.SyncInventory(ctx))
	}
	return ocfl.ObjectRoots(ctx, fsys, pth, eachRoot)
}
