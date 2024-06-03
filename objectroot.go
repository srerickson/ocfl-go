package ocfl

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

const (
	// HasNamaste indicates a NAMASTE object declaration file exists in the
	// object root directory
	HasNamaste objectRootFlag = 1 << iota
	// HasInventory indicates that an ObjectRoot includes an "inventory.json"
	// file
	HasInventory
	// HasSidecar indicates that an ObjectRoot includes an "inventory.json.*"
	// file (the inventory sidecar).
	HasSidecar
	// HasExtensions indicates that an ObjectRoot includes a directory
	// named "extensions"
	HasExtensions

	inventoryFile    = "inventory.json"
	sidecarPrefix    = inventoryFile + "."
	objectDeclPrefix = "0=" + NamasteTypeObject

	maxObjectRootStateInvalid = 8
)

var (
	ErrObjectExists          = fmt.Errorf("found existing OCFL object declaration: %w", fs.ErrExist)
	ErrObjectNamasteNotExist = fmt.Errorf("missing OCFL object declaration: %w", ErrNamasteNotExist)
)

// ObjectRoot represents an existing OCFL object root directory.
type ObjectRoot struct {
	// FS is the FS for accessing the object's contents
	FS FS
	// Path is the path in the FS for the object root directory
	Path string
	// State represents the contents of the object root directory.
	State *ObjectRootState
}

// GetObjectRoot reads the contents of directory dir in fsys, confirms that an
// OCFL Object declaration is present, and returns a new ObjectRoot reference
// with an initialized State. The object declaration is not read or fully
// validated. The returned ObjectRoot State will have the HasNamaste flag set,
// but other flags expected for a complete object root may not be set (e.g., if
// the inventory is missing).
func GetObjectRoot(ctx context.Context, fsys FS, dir string) (*ObjectRoot, error) {
	obj := &ObjectRoot{
		FS:   fsys,
		Path: dir,
	}
	if err := obj.SyncState(ctx); err != nil {
		return nil, err
	}
	if !obj.State.HasNamaste() {
		return nil, ErrObjectNamasteNotExist
	}
	return obj, nil
}

// SyncState reads the entries of of the object root directory
// and initializes obj.State
func (obj *ObjectRoot) SyncState(ctx context.Context) error {
	entries, err := obj.FS.ReadDir(ctx, obj.Path)
	if err != nil {
		return fmt.Errorf("reading object root directory: %w", err)
	}
	obj.State = ParseObjectRootEntries(entries)
	return nil
}

// ValidateNamaste reads and validates the contents of the OCFL object
// declaration in the object root.
func (obj ObjectRoot) ValidateNamaste(ctx context.Context) error {
	if obj.State == nil {
		if err := obj.SyncState(ctx); err != nil {
			return err
		}
	}
	if !obj.State.HasNamaste() {
		return ErrObjectNamasteNotExist
	}
	name := Namaste{Type: NamasteTypeObject, Version: obj.State.Spec}.Name()
	decl := path.Join(obj.Path, name)
	return ValidateNamaste(ctx, obj.FS, decl)
}

// ExtensionNames returns the names of directories in the
// object root's extensions directory.
// func (obj ObjectRoot) ExtensionNames(ctx context.Context) ([]string, error) {
// 	if obj.State != nil && !obj.State.HasExtensions() {
// 		return nil, nil
// 	}
// 	entries, err := obj.FS.ReadDir(ctx, path.Join(obj.Path, ExtensionsDir))
// 	if err != nil {
// 		if errors.Is(err, fs.ErrNotExist) {
// 			return nil, nil
// 		}
// 		return nil, err
// 	}
// 	names := make([]string, len(entries))
// 	for _, e := range entries {
// 		if !e.IsDir() {
// 			// return error?
// 			continue
// 		}
// 		names = append(names, e.Name())
// 	}
// 	return names, err
// }

// ObjectRootState represents the contents of an OCFL Object root directory.
type ObjectRootState struct {
	Spec        Spec           // the OCFL spec from the object's NAMASTE declaration file
	VersionDirs VNums          // versions directories found in the object directory
	SidecarAlg  string         // digest algorithm decl by the inventory sidecar
	Invalid     []string       // non-conforming entries found in the object root (max=8)
	Flags       objectRootFlag // represents various boolean attributes
}

type objectRootFlag uint8

// ParseObjectRootEntries returns a new ObjectRootState based on contents of an
// object root directory
func ParseObjectRootEntries(entries []fs.DirEntry) *ObjectRootState {
	state := &ObjectRootState{}
	addNonConfoming := func(name string) {
		if len(state.Invalid) < maxObjectRootStateInvalid {
			state.Invalid = append(state.Invalid, name)
		}
	}
	for _, e := range entries {
		name := e.Name()
		switch {
		case e.IsDir():
			var v VNum
			switch {
			case name == ExtensionsDir:
				state.Flags |= HasExtensions
			case ParseVNum(name, &v) == nil:
				state.VersionDirs = append(state.VersionDirs, v)
			default:
				addNonConfoming(name)
			}
		case validFileType(e.Type()):
			switch {
			case name == inventoryFile:
				state.Flags |= HasInventory
			case strings.HasPrefix(name, sidecarPrefix):
				if state.HasSidecar() {
					// duplicate sidecar-like file
					addNonConfoming(name)
					break
				}
				state.SidecarAlg = strings.TrimPrefix(name, sidecarPrefix)
				state.Flags |= HasSidecar
			case strings.HasPrefix(name, objectDeclPrefix):
				if state.HasNamaste() {
					// duplicate namaste
					addNonConfoming(name)
					break
				}
				decl, err := ParseNamaste(name)
				if err != nil {
					addNonConfoming(name)
					break
				}
				state.Spec = decl.Version
				state.Flags |= HasNamaste
			default:
				addNonConfoming(name)
			}
		default:
			// invalid file mode type
			addNonConfoming(name)
		}
	}
	return state
}

// HasNamaste returns true if the object's FoundDeclaration flag is set
func (state ObjectRootState) HasNamaste() bool {
	return state.Flags&HasNamaste > 0
}

// HasInventory returns true if the object's FoundInventory flag is set
func (state ObjectRootState) HasInventory() bool {
	return state.Flags&HasInventory > 0
}

// HasSidecar returns true if the object's FoundSidecar flag is set
func (state ObjectRootState) HasSidecar() bool {
	return state.Flags&HasSidecar > 0
}

// HasExtensions returns true if the object's HasExtensions flag is set
func (state ObjectRootState) HasExtensions() bool {
	return state.Flags&HasExtensions > 0
}

func (state ObjectRootState) HasVersionDir(dir VNum) bool {
	return slices.Contains(state.VersionDirs, dir)
}

// ObjectRootsFS is used to iterate over object roots
type ObjectRootsFS interface {
	// ObjectRoots searches root and its subdirectories for OCFL object declarations
	// and and returns an iterator that yields each object root it finds. The
	// *ObjectRoot passed to yield is confirmed to have an object declaration, but
	// no other validation checks are made.
	ObjectRoots(ctx context.Context, dir string) ObjectRootSeq
}

// ObjectRootSeq is an iterator that yieldss ObjectRoot references; it is returned
// by ObjectRoots()
type ObjectRootSeq func(yield func(*ObjectRoot, error) bool)

// ObjectRoots searches root and its subdirectories for OCFL object declarations
// and returns an iterator that yields each object root it finds. The
// *ObjectRoot passed to yield is confirmed to have an object declaration, but
// no other validation checks are made.
func ObjectRoots(ctx context.Context, fsys FS, dir string) ObjectRootSeq {
	if iterFS, ok := fsys.(ObjectRootsFS); ok {
		return iterFS.ObjectRoots(ctx, dir)
	}
	return func(yield func(*ObjectRoot, error) bool) {
		walkObjectRoots(ctx, fsys, dir, yield)
	}
}

func walkObjectRoots(ctx context.Context, fsys FS, dir string, yield func(*ObjectRoot, error) bool) bool {
	entries, err := fsys.ReadDir(ctx, dir)
	if err != nil {
		yield(nil, err)
		return false
	}
	objRoot := &ObjectRoot{
		FS:    fsys,
		Path:  dir,
		State: ParseObjectRootEntries(entries),
	}
	if objRoot.State.HasNamaste() {
		return yield(objRoot, nil)
	}
	for _, e := range entries {
		if e.IsDir() {
			subdir := path.Join(dir, e.Name())
			if !walkObjectRoots(ctx, fsys, subdir, yield) {
				return false
			}
		}
	}
	return true
}
