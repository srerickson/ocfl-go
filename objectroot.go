package ocfl

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/srerickson/ocfl-go/internal/walkdirs"
)

const (
	inventoryFile = "inventory.json"
	maxNonConform = 8
)

var (
	ErrObjectNotFound = fmt.Errorf("missing object declaration: %w", ErrNoNamaste)
	ErrObjectExists   = fmt.Errorf("existing OCFL object declaration: %w", fs.ErrExist)
)

// GetObjectRoot reads the contents of directory dir in fsys, confirms that an
// OCFL Object declaration is present, and returns a new ObjectRoot reference
// based on the directory contents. If the directory cannot be read or a
// declaration is not found, ErrObjectNotFound is returned. Note, the object
// declaration is not read or fully validated. The returned ObjectRoot will have
// the FoundNamaste flag set, but other flags expected for a complete object
// root may not be set (e.g., if the inventory is missing).
func GetObjectRoot(ctx context.Context, fsys FS, dir string) (*ObjectRoot, error) {
	entries, err := fsys.ReadDir(ctx, dir)
	if err != nil {
		return nil, err
	}
	obj := NewObjectRoot(fsys, dir, entries)
	if !obj.HasNamaste() {
		return nil, fmt.Errorf("missing object declaration: %w", ErrNoNamaste)
	}
	return obj, nil
}

// NewObjectRoot constructs an ObjectRoot for the directory dir in fsys using
// the given fs.DirEntry slice as dir's contents. The returned ObjectRoot may
// be invalid.
func NewObjectRoot(fsys FS, dir string, entries []fs.DirEntry) *ObjectRoot {
	// set zero values for everything except FS and Path
	obj := &ObjectRoot{
		FS:   fsys,
		Path: dir,
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if name == ExtensionsDir {
				obj.Flags |= HasExtensions
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
				obj.Flags |= HasInventory
				continue
			}
			scPrefix := inventoryFile + "."
			if strings.HasPrefix(name, scPrefix) {
				obj.SidecarAlg = strings.TrimPrefix(name, scPrefix)
				obj.Flags |= HasSidecar
				continue
			}
			if decl, err := ParseNamaste(name); err == nil {
				if decl.Type == NamasteTypeObject && !obj.HasNamaste() {
					obj.Spec = decl.Version
					obj.Flags |= HasNamaste
					continue
				}
			}
		}
		// entry doesn't conform to OCFL spec
		if len(obj.NonConform) < maxNonConform {
			obj.NonConform = append(obj.NonConform, name)
		}
	}
	return obj
}

// ObjectRoot represents an existing OCFL object root directory. Instances are
// typically created with functions like GetObjectRoot().
type ObjectRoot struct {
	FS          FS       // the FS where the object is stored
	Path        string   // object path in FS
	Spec        Spec     // the OCFL spec from the object's NAMASTE declaration
	VersionDirs VNums    // versions directories found in the object directory
	SidecarAlg  string   // digest algorithm declared by the inventory sidecar
	NonConform  []string // non-conforming entries found in the object root (max=8)
	Flags       ObjectRootFlag
}

type ObjectRootFlag int

const (
	// HasNamaste indicates that an ObjectRoot has been initialized
	// and an object declaration file is confirmed to exist in the object's root
	// directory
	HasNamaste ObjectRootFlag = 1 << iota
	// HasInventory indicates that an ObjectRoot includes an "inventory.json"
	// file
	HasInventory
	// HasSidecar indicates that an ObjectRoot includes an "inventory.json.*"
	// file (the inventory sidecar).
	HasSidecar
	// HasExtensions indicates that an ObjectRoot includes a directory
	// named "extensions"
	HasExtensions
)

// ValidateNamaste reads and validates the contents of the OCFL object
// declaration in the object root.
func (obj ObjectRoot) ValidateNamaste(ctx context.Context) error {
	if !obj.HasNamaste() {
		return ErrNoNamaste
	}
	pth := path.Join(obj.Path, Namaste{Type: NamasteTypeObject, Version: obj.Spec}.Name())
	return ReadNamaste(ctx, obj.FS, pth)
}

// HasNamaste returns true if the object's FoundDeclaration flag is set
func (obj ObjectRoot) HasNamaste() bool {
	return obj.Flags&HasNamaste > 0
}

// HasInventory returns true if the object's FoundInventory flag is set
func (obj ObjectRoot) HasInventory() bool {
	return obj.Flags&HasInventory > 0
}

// HasSidecar returns true if the object's FoundSidecar flag is set
func (obj ObjectRoot) HasSidecar() bool {
	return obj.Flags&HasSidecar > 0
}

// HasExtensions returns true if the object's HasExtensions flag is set
func (obj ObjectRoot) HasExtensions() bool {
	return obj.Flags&HasExtensions > 0
}

func (obj ObjectRoot) HasVersionDir(dir VNum) bool {
	return slices.Contains(obj.VersionDirs, dir)
}

// ObjectRootIterator is used to iterate over object roots
type ObjectRootIterator interface {
	// ObjectRoots searches root and its subdirectories for OCFL object declarations
	// and calls fn for each object root it finds. The *ObjectRoot passed to fn is
	// confirmed to have an object declaration, but no other validation checks are
	// made.
	ObjectRoots(ctx context.Context, sel PathSelector, fn func(obj *ObjectRoot) error) error
}

// ObjectRoots searches root and its subdirectories for OCFL object declarations
// and calls fn for each object root it finds. The *ObjectRoot passed to fn is
// confirmed to have an object declaration, but no other validation checks are
// made.
func ObjectRoots(ctx context.Context, fsys FS, sel PathSelector, fn func(*ObjectRoot) error) error {
	if iterFS, ok := fsys.(ObjectRootIterator); ok {
		return iterFS.ObjectRoots(ctx, sel, fn)
	}
	walkFn := func(name string, entries []fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		objRoot := NewObjectRoot(fsys, name, entries)
		if objRoot.HasNamaste() {
			if err := fn(objRoot); err != nil {
				return err
			}
			// don't walk object subdirectories
			return walkdirs.ErrSkipDirs
		}
		return nil
	}
	return walkdirs.WalkDirs(ctx, fsys, sel.Path(), sel.SkipDir, walkFn, 0)
}
