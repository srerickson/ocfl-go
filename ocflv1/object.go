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
	// backend filesystem
	fsys ocfl.FS
	// path to object root
	rootDir string
	// cache of object info
	info *ocfl.ObjectSummary
	// cache of inventory
	inv *Inventory
	// cache of inventory sidecar
	sidecarDigest string
}

// GetObject returns a new Object with loaded inventory.
func GetObject(ctx context.Context, fsys ocfl.FS, root string) (*Object, error) {
	inf, err := ocfl.ReadObjectSummary(ctx, fsys, root)
	if err != nil {
		return nil, fmt.Errorf("reading object: %w", err)
	}
	if inf.Declaration.Type != ocfl.DeclObject {
		return nil, fmt.Errorf("declared type: %s: %w", inf.Declaration.Type, ErrNotObject)

	}
	if !ocflVerSupported[inf.Declaration.Version] {
		return nil, fmt.Errorf("%s: %w", inf.Declaration.Version, ErrOCFLVersion)
	}
	err = ocfl.ValidateDeclaration(ctx, fsys, path.Join(root, inf.Declaration.Name()))
	if err != nil {
		return nil, err
	}
	obj := &Object{
		fsys:    fsys,
		rootDir: root,
		info:    inf,
	}
	return obj, nil
}

// Root returns the object's FS and root directory. The root directory is a path
// relative to the object's ocfl.FS.
func (obj *Object) Root() (ocfl.FS, string) {
	return obj.fsys, obj.rootDir
}

// Summary returns a description of the object's top-level directory contents. The
// value is cached: subsequent calls return the same value.
func (obj *Object) Summary(ctx context.Context) (*ocfl.ObjectSummary, error) {
	if obj.info != nil {
		return obj.info, nil
	}
	inf, err := ocfl.ReadObjectSummary(ctx, obj.fsys, obj.rootDir)
	if err != nil {
		return nil, err
	}
	obj.info = inf
	return obj.info, nil
}

// Inventory returns the root inventory for the object. The first time
// Inventory() is called, the inventory is downloaded, validated, and returned.
// An error is returned if the inventory cannot be read or it is invalid.  The
// value is cached: subsequent calls return the same value.
func (obj *Object) Inventory(ctx context.Context) (*Inventory, error) {
	if obj.inv != nil {
		return obj.inv, nil
	}
	info, err := obj.Summary(ctx)
	if err != nil {
		return nil, err
	}
	name := path.Join(obj.rootDir, inventoryFile)
	alg, err := digest.Get(info.Algorithm)
	if err != nil {
		return nil, fmt.Errorf("reading inventory: %w", err)
	}
	inv, results := ValidateInventory(ctx, obj.fsys, name, alg)
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
	inf, err := obj.Summary(ctx)
	if err != nil {
		return "", err
	}
	sidecarFile := path.Join(obj.rootDir, inventoryFile+"."+inf.Algorithm)
	sidecar, err := readInventorySidecar(ctx, obj.fsys, sidecarFile)
	if err != nil {
		return "", err
	}
	obj.sidecarDigest = sidecar
	return obj.sidecarDigest, nil
}

// Validate validations the object using the given validation Options
func (obj *Object) Validate(ctx context.Context, opts ...ValidationOption) *validation.Result {
	_, r := ValidateObject(ctx, obj.fsys, obj.rootDir, opts...)
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
