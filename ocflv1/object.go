package ocflv1

import (
	"context"
	"fmt"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl/namaste"
	"github.com/srerickson/ocfl/object"
)

// Object represents an existing OCFL v1.x object
type Object struct {
	// backend filesystem
	fsys fs.FS
	// path to object root
	rootDir string
	// cache of object info
	info *object.Info
}

// GetObject returns a new Object with loaded inventory.
func GetObject(ctx context.Context, fsys fs.FS, root string) (*Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	inf, err := object.ReadInfo(ctx, fsys, root)
	if err != nil {
		return nil, fmt.Errorf("reading object: %w", err)
	}
	if inf.Declaration.Type != namaste.ObjectType {
		return nil, fmt.Errorf("declared type: %s: %w", inf.Declaration.Type, object.ErrNotObject)

	}
	if !ocflVerSupported[inf.Declaration.Version] {
		return nil, fmt.Errorf("%s: %w", inf.Declaration.Version, object.ErrOCFLVersion)
	}
	err = namaste.Validate(ctx, fsys, path.Join(root, inf.Declaration.Name()))
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

func (obj *Object) Info(ctx context.Context) (*object.Info, error) {
	if obj.info == nil {
		var err error
		obj.info, err = object.ReadInfo(ctx, obj.fsys, obj.rootDir)
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
