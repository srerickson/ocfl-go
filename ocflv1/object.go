package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/srerickson/ocfl/digest"
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
	// cache of object inventory
	inventory *Inventory
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
	if obj.inventory == nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		invReader, err := obj.fsys.Open(path.Join(obj.rootDir, inventoryFile))
		if err != nil {
			return nil, err
		}
		defer invReader.Close()
		obj.inventory = &Inventory{}
		_, err = object.ReadDigestInventory(ctx, invReader, obj.inventory, info.Algorithm)
		if err != nil {
			return nil, fmt.Errorf("decode inventory: %w", err)
		}
		// set digest?
	}
	return obj.inventory, nil
}

func (obj *Object) ID(ctx context.Context) (string, error) {
	inv, err := obj.Inventory(ctx)
	if err != nil {
		return "", err
	}
	return inv.ID, nil
}

func (obj *Object) Manifest(ctx context.Context) (*digest.Map, error) {
	inv, err := obj.Inventory(ctx)
	if err != nil {
		return nil, err
	}
	return inv.Manifest, nil
}

func (obj *Object) DigestAlgorithm(ctx context.Context) (digest.Alg, error) {
	info, err := obj.Info(ctx)
	if err != nil {
		return digest.Alg{}, err
	}
	return info.Algorithm, nil
}

func (obj *Object) Head(ctx context.Context) (object.VNum, error) {
	inv, err := obj.Inventory(ctx)
	if err != nil {
		return object.VNum{}, err
	}
	err = inv.Head.Valid()
	if err != nil {
		return object.VNum{}, err
	}
	return inv.Head, nil
}

// If v is the zero value VNum, the object's head version is used, if available.
func (obj *Object) Version(ctx context.Context, v object.VNum) (*object.VState, error) {
	inv, err := obj.Inventory(ctx)
	if err != nil {
		return nil, err
	}
	return inv.VState(v), nil
}

func (obj *Object) Versions(ctx context.Context) (object.VNumSeq, error) {
	inv, err := obj.Inventory(ctx)
	if err != nil {
		return nil, err
	}
	return inv.VNums(), nil
}

// If v is the zero value VNum, the object's head version is used, if available.
func (obj *Object) GetContent(ctx context.Context, v object.VNum, logical string) (string, error) {
	state, err := obj.Version(ctx, v)
	if err != nil {
		return "", err
	}
	entries := state.State[logical]
	if len(entries) == 0 {
		return "", fmt.Errorf("not found: %s/%s", v, logical)
	}
	return entries[0], nil
}

// EachContent runs the funcion do for each content file in version directory v.
// The string passed to do is the path of existing file relative to the object
// root. If v is the zero value VNum, the object's head version is used, if
// available.
func (obj *Object) EachContent(ctx context.Context, v object.VNum, do func(string) error) error {
	inv, err := obj.Inventory(ctx)
	if err != nil {
		return err
	}
	if v.Num() == 0 {
		v, err = obj.Head(ctx)
		if err != nil {
			return err
		}
	}
	// FIXME: we don't need this initial stat. Can check error from walk
	// if content doesn't exist.
	vcDir := path.Join(obj.rootDir, v.String(), inv.ContentDirectory)
	info, err := fs.Stat(obj.fsys, vcDir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("each content: %w", err)
		}
		return nil
	}
	if !info.IsDir() {
		return fmt.Errorf(`each content: %s not a directory`, vcDir)
	}
	f := func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			return do(strings.TrimPrefix(p, obj.rootDir+"/"))
		}
		return nil
	}
	return fs.WalkDir(obj.fsys, vcDir, f)
}

// EachLogical runs the function do for each logical path in the state of
// version v. The strings passed to do are the logical path and a corresponding
// content path. If do returns an non-nil error, no additional calls to do are
// made. If v is the zero value VNum, the object's head version is used, if
// available.
// func (obj *Object) EachLogical(ctx context.Context, v object.VNum, do func(logical, content string) error) error {
// 	manifest, err := obj.Manifest(ctx)
// 	if err != nil {
// 		return err
// 	}
// 	version, err := obj.Version(ctx, v)
// 	if err != nil {
// 		return err
// 	}
// 	for p, d := range version.State.AllPaths() {
// 		entries := manifest.DigestPaths(d)
// 		if len(entries) == 0 {
// 			return fmt.Errorf("digest not found in manifest: %s", d)
// 		}
// 		err := do(p, entries[0])
// 		if err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }
