package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
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

// Object represents an existing OCFL v1.x object
type Object struct {
	ocfl.ObjectRoot

	// cache of inventory
	inv *Inventory
	// cache of inventory sidecar
	sidecarDigest string
}

// GetObject returns a new Object
func GetObject(ctx context.Context, fsys ocfl.FS, p string) (*Object, error) {
	root, err := ocfl.GetObjectRoot(ctx, fsys, p)
	if err != nil {
		return nil, err
	}
	if !ocflVerSupported[root.Spec] {
		return nil, fmt.Errorf("%s: %w", root.Spec, ErrOCFLVersion)
	}
	if err = root.ValidateDeclaration(ctx); err != nil {
		return nil, err
	}
	return &Object{ObjectRoot: *root}, nil
}

// Root returns the object's FS and root directory. The root directory is a path
// relative to the object's ocfl.FS.
func (obj *Object) Root() (ocfl.FS, string) {
	return obj.ObjectRoot.FS, obj.ObjectRoot.Path
}

// Inventory returns the root inventory for the object. The first time
// Inventory() is called, the inventory is downloaded, validated, and returned.
// An error is returned if the inventory cannot be read or it is invalid.  The
// value is cached: subsequent calls return the same value.
func (obj *Object) Inventory(ctx context.Context) (*Inventory, error) {
	if obj.inv != nil {
		return obj.inv, nil
	}
	name := path.Join(obj.Path, inventoryFile)
	alg, err := digest.Get(obj.Algorithm)
	if err != nil {
		return nil, fmt.Errorf("reading inventory: %w", err)
	}
	inv, results := ValidateInventory(ctx, obj.FS, name, alg)
	if err := results.Err(); err != nil {
		return nil, fmt.Errorf("reading inventory: %w", err)
	}
	obj.inv = inv
	return obj.inv, nil
}

// InventorySidecar returns the digest stored in the root inventory sidecar
// file.  The value is cached: subsequent calls return the same value.
func (obj *Object) InventorySidecar(ctx context.Context) (string, error) {
	if obj.sidecarDigest != "" {
		return obj.sidecarDigest, nil
	}

	sidecarFile := path.Join(obj.Path, inventoryFile+"."+obj.Algorithm)
	sidecar, err := readInventorySidecar(ctx, obj.FS, sidecarFile)
	if err != nil {
		return "", err
	}
	obj.sidecarDigest = sidecar
	return obj.sidecarDigest, nil
}

// Validate validations the object using the given validation Options
func (obj *Object) Validate(ctx context.Context, opts ...ValidationOption) *validation.Result {
	_, r := ValidateObject(ctx, obj.FS, obj.Path, opts...)
	return r
}

// NewStage returns a Stage based on the specified version of the object. If ver
// is the empty value, the head version is used.
func (obj *Object) NewStage(ctx context.Context, ver ocfl.VNum, opts ...ocfl.StageOption) (*ocfl.Stage, error) {
	inv, err := obj.Inventory(ctx)
	if err != nil {
		return nil, err
	}
	idx, err := inv.Index(ver.Num())
	if err != nil {
		return nil, err
	}
	alg, err := digest.Get(inv.DigestAlgorithm)
	if err != nil {
		return nil, err
	}
	opts = append(opts, ocfl.StageIndex(idx))
	return ocfl.NewStage(alg, opts...), nil
}
