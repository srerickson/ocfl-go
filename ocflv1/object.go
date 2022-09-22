package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/srerickson/ocfl"
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
	info *ocfl.ObjInfo
}

// GetObject returns a new Object with loaded inventory.
func GetObject(ctx context.Context, fsys ocfl.FS, root string) (*Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inf, err := ocfl.ReadObjInfo(ctx, fsys, root)
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

func (obj *Object) Info(ctx context.Context) (*ocfl.ObjInfo, error) {
	if obj.info == nil {
		var err error
		obj.info, err = ocfl.ReadObjInfo(ctx, obj.fsys, obj.rootDir)
		if err != nil {
			return nil, err
		}
	}
	return obj.info, nil
}

func (obj *Object) Inventory(ctx context.Context) (*Inventory, error) {
	info, err := obj.Info(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inv, err := ValidateInventory(ctx, &ValidateInventoryConf{
		FS:              obj.fsys,
		Name:            path.Join(obj.rootDir, inventoryFile),
		DigestAlgorithm: info.Algorithm,
	})
	if err != nil {
		return nil, fmt.Errorf("reading inventory: %w", err)
	}
	return inv, err
}

// InventorySidecar returns the digest stored in the root inventory sidecar
// file.
func (obj *Object) InventorySidecar(ctx context.Context) (string, error) {
	inf, err := obj.Info(ctx)
	if err != nil {
		return "", err
	}
	sidecar := inventoryFile + "." + inf.Algorithm.ID()
	reader, err := obj.fsys.OpenFile(ctx, path.Join(obj.rootDir, sidecar))
	if err != nil {
		return "", err
	}
	defer reader.Close()
	return readInventorySidecar(ctx, reader)
}

func (obj *Object) Validate(ctx context.Context) error {
	return ValidateObject(ctx, obj.fsys, obj.rootDir, nil)
}
