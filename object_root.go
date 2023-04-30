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

var (
	ErrObjectNotFound = errors.New("OCFL object declaration not found")
	ErrObjectExists   = errors.New("OCFL object declaration already exists")
)

// GetObjectRoot reads the contents of directory dir in fsys, confirms that an
// OCFL Object declaration is present, and returns a new ObjectRoot reference
// based on the directory contents. If the directory cannot be ready or a
// declaration is not found, ErrObjectNotFound is returned. Note, the object
// declaration is not read or fully validated. The returned ObjectRoot will have
// the FoundDeclaration flag set, but other flags expected for a complete object
// root may not be set (e.g., if the inventory is missing).
func GetObjectRoot(ctx context.Context, fsys FS, dir string) (*ObjectRoot, error) {
	entries, err := fsys.ReadDir(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrObjectNotFound, err.Error())
	}
	obj := NewObjectRoot(fsys, dir, entries)
	if !obj.HasDeclaration() {
		return nil, fmt.Errorf("%w: %s", ErrObjectNotFound, ErrDeclMissing.Error())
	}
	return obj, nil
}

// InitObjectRoot creates an OCFL object declaration file in the directory dir
// if one does not exist and the directory's contents do not include any
// non-conforming entries. If the directory does not exist, it is created along
// with all parent directories. If a declaration file exists, a fully
// initialized ObjectRoot is returned (same as GetObjectRoot) along with
// ErrObjectExists. In the latter case, the returned ObjectRoot's Spec value may
// not match the spec argument. If the directory includes any files or
// directories that do not conform to an OCFL object, an error is returned.
func InitObjectRoot(ctx context.Context, fsys WriteFS, dir string, spec Spec) (*ObjectRoot, error) {
	entries, err := fsys.ReadDir(ctx, dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	obj := NewObjectRoot(fsys, dir, entries)
	if obj.HasDeclaration() {
		return obj, ErrObjectExists
	}
	if len(obj.NonConform) > 0 {
		return nil, errors.New("directory includes non-conforming entries")
	}
	decl := Declaration{Type: DeclObject, Version: spec}
	if err := WriteDeclaration(ctx, fsys, dir, decl); err != nil {
		return nil, err
	}
	obj.Spec = spec
	obj.Flags |= FoundDeclaration
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
				if decl.Type == DeclObject && !obj.HasDeclaration() {
					obj.Spec = decl.Version
					obj.Flags |= FoundDeclaration
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
	Algorithm   string   // digest algorithm declared by the inventory sidecar
	NonConform  []string // non-conforming entries found in the object root (max=8)
	Flags       ObjectRootFlag
}

type ObjectRootFlag int

const (
	// FoundDeclaration indicates that an ObjectRoot has been initialized
	// and an object declaration file is confirmed to exist in the object's root
	// directory
	FoundDeclaration ObjectRootFlag = 1 << iota
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
func (obj ObjectRoot) ValidateDeclaration(ctx context.Context) error {
	if !obj.HasDeclaration() {
		return ErrDeclMissing
	}
	pth := path.Join(obj.Path, Declaration{Type: DeclObject, Version: obj.Spec}.Name())
	return ValidateDeclaration(ctx, obj.FS, pth)
}

// HasDeclaration returns true if the object's FoundDeclaration flag is set
func (obj ObjectRoot) HasDeclaration() bool {
	return obj.Flags&FoundDeclaration > 0
}

// HasInventory returns true if the object's FoundInventory flag is set
func (obj ObjectRoot) HasInventory() bool {
	return obj.Flags&FoundInventory > 0
}

// HasSidecar returns true if the object's FoundSidecar flag is set
func (obj ObjectRoot) HasSidecar() bool {
	return obj.Flags&FoundSidecar > 0
}

func (obj ObjectRoot) HasVersionDir(dir VNum) bool {
	for _, v := range obj.VersionDirs {
		if v == dir {
			return true
		}
	}
	return false
}
