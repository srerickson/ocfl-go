package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/logging"
	"github.com/srerickson/ocfl-go/validation"
)

var (
	ErrOCFLVersion        = errors.New("unsupported OCFL version")
	ErrInventoryNotExist  = fmt.Errorf("missing inventory file: %w", fs.ErrNotExist)
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
		return nil, ErrInventoryNotExist
	}
	obj := &Object{ObjectRoot: *root}
	if err := obj.SyncInventory(ctx); err != nil {
		return nil, err
	}
	return obj, nil
}

// SyncInventory downloads and validates the object's root inventory. If
// successful the object's Inventory value is updated.
func (obj *Object) SyncInventory(ctx context.Context) error {
	name := path.Join(obj.Path, inventoryFile)
	nolog := ValidationLogger(logging.DisabledLogger())
	inv, results := ValidateInventory(ctx, obj.FS, name, nolog)
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

func (obj *Object) Stage(i int) (*ocfl.Stage, error) {
	version := obj.Inventory.Version(i)
	if version == nil {
		return nil, ErrVersionNotFound
	}
	state, err := version.State.Normalize()
	if err != nil {
		return nil, err
	}
	return &ocfl.Stage{
		State:           state,
		DigestAlgorithm: obj.Inventory.DigestAlgorithm,
		ContentSource:   obj,
		FixitySource:    obj,
	}, nil
}

// GetContent implements ocfl.ContentSource for Object
func (obj *Object) GetContent(digest string) (ocfl.FS, string) {
	if obj.Inventory.Manifest == nil {
		return nil, ""
	}
	paths := obj.Inventory.Manifest[digest]
	if len(paths) < 1 {
		return nil, ""
	}
	return obj.FS, path.Join(obj.ObjectRoot.Path, paths[0])
}

// Fixity implements ocfl.FixitySource for Object
func (obj *Object) GetFixity(digest string) ocfl.DigestSet {
	return obj.Inventory.GetFixity(digest)
}

// Objects returns a function iterator that yields Objects
// found in dir and its subdirectories
func Objects(ctx context.Context, fsys ocfl.FS, dir string) ObjectSeq {
	return func(yieldObject func(*Object, error) bool) {
		objectRootIter := ocfl.ObjectRoots(ctx, fsys, dir)
		objectRootIter(func(objRoot *ocfl.ObjectRoot, err error) bool {
			var obj *Object
			if objRoot != nil {
				obj = &Object{ObjectRoot: *objRoot}
			}
			if err != nil && !yieldObject(obj, err) {
				return false
			}
			return yieldObject(obj, obj.SyncInventory(ctx))
		})
	}
}

type ObjectSeq func(yield func(*Object, error) bool)
