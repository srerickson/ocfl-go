package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

const (
	inventoryFile = "inventory.json"
	extensionsDir = "extensions"
	maxNonConform = 8
)

var ErrNotObjectRoot = errors.New("not an OCFL object root directory")
var ErrObjectRootMode = errors.New("file in object root has invalid mode")

// GetObjectRoot reads the contents of directory dir in fsys, confirms that an
// OCFL Object declaration is present, and returns a new ObjectRoot reference
// based on the directory contents. If the directory cannot be ready or a
// declaration is not found, an error is returned. Note, the object declaration
// is not read or fully validated. The returned ObjectRoot may be invalid.
func GetObjectRoot(ctx context.Context, fsys FS, dir string) (*ObjectRoot, error) {
	entries, err := fsys.ReadDir(ctx, dir)
	if err != nil {
		return nil, err
	}
	return NewObjectRoot(fsys, dir, entries)
}

// NewObjectRoot constructs an ObjectRoot for the directory dir in fsys using
// the given fs.DirEntry slice as dir's contents. If entries does not include
// an entry for the object declaration file, an error is returned.
func NewObjectRoot(fsys FS, dir string, entries []fs.DirEntry) (*ObjectRoot, error) {
	// set zero values for everything except FS and Path
	obj := &ObjectRoot{
		FS:   fsys,
		Path: dir,
	}
	for _, e := range entries {
		if !e.IsDir() && !e.Type().IsRegular() {
			err := fmt.Errorf("%w: %s", ErrObjectRootMode, e.Name())
			return nil, err
		}
		name := e.Name()
		if e.IsDir() {
			if name == extensionsDir {
				obj.Flags |= FoundExtensions
				continue
			}
			var v VNum
			if err := ParseVNum(name, &v); err == nil {
				obj.VersionDirs = append(obj.VersionDirs, v)
				continue
			}
		}
		if e.Type().IsRegular() { // regular file
			if name == inventoryFile {
				obj.Flags |= FoundInventory
				continue
			}
			scPrefix := inventoryFile + "."
			if strings.HasPrefix(name, scPrefix) {
				obj.Algorithm = strings.TrimPrefix(name, scPrefix)
				obj.Flags |= FoundSidecar
				continue
			}
			var decl Declaration
			if err := ParseDeclaration(name, &decl); err == nil {
				if obj.Flags&FoundDeclaration > 0 {
					// multiple declaration files
					return nil, ErrDeclMultiple
				}
				if decl.Type != DeclObject {
					// not an object declaration
					return nil, ErrNotObjectRoot
				}
				obj.Spec = decl.Version
				obj.Flags |= FoundDeclaration
				continue
			}
		}
		// entry doesn't conform to OCFL spec
		if len(obj.NonConform) < maxNonConform {
			obj.NonConform = append(obj.NonConform, name)
		}
	}
	if obj.Flags&FoundDeclaration == 0 {
		return nil, ErrDeclMissing
	}
	return obj, nil
}

// ObjectRoot represents an existing OCFL object root directory. Instances are
// typically created with functions like GetObjectRoot().
type ObjectRoot struct {
	FS          FS       // the FS where the object is stored
	Path        string   // object path in FS
	Spec        Spec     // the OCFL spec from the object's NAMASTE declaration
	VersionDirs VNums    // versions directories found in the object directory
	Algorithm   string   // digest algorithm declared by the inventory sidecar
	NonConform  []string // non-conforming entries found in the object root (max=8)
	Flags       ObjectRootFlag
}

type ObjectRootFlag int

const (
	// FoundDeclaration indicates that an ObjectRoot has been initialized
	// and an object declaration file is confirmed to exist in the object's root
	// directory
	FoundDeclaration = 1 << iota
	// FoundInventory indicates that an ObjectRoot includes an "inventory.json"
	// file
	FoundInventory
	// FoundSidecar indicates that an ObjectRoot includes an "inventory.json.*"
	// file (the inventory sidecar).
	FoundSidecar
	// FoundExtensions indicates that an ObjectRoot includes a directory
	// named "extensions"
	FoundExtensions
)

// ValidateDeclaration reads and validates the contents of the OCFL object
// declaration in the object root.
func (obj *ObjectRoot) ValidateDeclaration(ctx context.Context) error {
	if obj.Flags&FoundDeclaration == 0 {
		return ErrNotObjectRoot
	}
	pth := path.Join(obj.Path, Declaration{Type: DeclObject, Version: obj.Spec}.Name())
	return ValidateDeclaration(ctx, obj.FS, pth)
}
