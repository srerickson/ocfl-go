package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl-go"
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

// GetObject returns an existing oject at dir in fsys. It returns an error if
// dir doesn't exist or doesn't include an object declaration file, or if the
// contents of the root inventory can't be unmarshalled into an Inventory value.
// Neither the object root or the inventory are fully validated.
func GetObject(ctx context.Context, fsys ocfl.FS, dir string) (*Object, error) {
	root, err := ocfl.GetObjectRoot(ctx, fsys, dir)
	if err != nil {
		return nil, err
	}
	if !ocflVerSupported[root.State.Spec] {
		return nil, fmt.Errorf("%s: %w", root.State.Spec, ErrOCFLVersion)
	}
	if !root.State.HasInventory() {
		// what is the best error to use here?
		return nil, ErrInventoryNotExist
	}
	obj := &Object{ObjectRoot: *root}
	if err := obj.SyncInventory(ctx); err != nil {
		return nil, err
	}
	return obj, nil
}

// SyncInventory reads and unmarshals the object's existing root inventory into
// obj.Inventory.
func (obj *Object) SyncInventory(ctx context.Context) error {
	var newInv Inventory
	if err := obj.ObjectRoot.UnmarshalInventory(ctx, ".", &newInv); err != nil {
		return err
	}
	obj.Inventory = newInv
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

// Stage returns an ocfl.Stage based on the specified version index.
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

// GetFixity implements ocfl.FixitySource for Object
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
