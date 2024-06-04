package ocfl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
)

const (
	// HasNamaste indicates that an object root directory includes a NAMASTE
	// object declaration file
	HasNamaste objectRootFlag = 1 << iota
	// HasInventory indicates that an object root includes an "inventory.json"
	// file
	HasInventory
	// HasSidecar indicates that an object root includes an "inventory.json.*"
	// file (the inventory sidecar).
	HasSidecar
	// HasExtensions indicates that an object root includes a directory
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
	if err := obj.checkState(ctx); err != nil {
		return nil, err
	}
	return obj, nil
}

// SyncState reads the entries of the object root directory and initializes the
// ObjectRoot's state.
func (obj *ObjectRoot) SyncState(ctx context.Context) error {
	entries, err := obj.FS.ReadDir(ctx, obj.Path)
	if err != nil {
		return fmt.Errorf("reading object root directory: %w", err)
	}
	obj.State = ParseObjectRootDir(entries)
	return nil
}

// ValidateNamaste reads and validates the contents of the OCFL object
// declaration in the object root. The ObjectRoot's State is initialized if it
// is nil.
func (obj *ObjectRoot) ValidateNamaste(ctx context.Context) error {
	if err := obj.checkState(ctx); err != nil {
		return err
	}
	decl := Namaste{Type: NamasteTypeObject, Version: obj.State.Spec}.Name()
	return ValidateNamaste(ctx, obj.FS, path.Join(obj.Path, decl))
}

// ExtensionNames returns the names of directories in the object root's
// extensions directory. The ObjectRoot's State is initialized if it is
// nil. If the object root does not include an object declaration, an error
// is returned. If object root does not include an extensions directory both
// return values are nil.
func (obj ObjectRoot) ExtensionNames(ctx context.Context) ([]string, error) {
	if err := obj.checkState(ctx); err != nil {
		return nil, err
	}
	if !obj.State.HasExtensions() {
		return nil, nil
	}
	entries, err := obj.ReadDir(ctx, ExtensionsDir)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			// if the extensions directory includes non-directory
			// entries, should we return an error?
			continue
		}
		names = append(names, e.Name())
	}
	return names, err
}

// UnmarshalInventory unmarshals the contents of the object root's
// inventory.json file into the value pointed to by v.
func (obj ObjectRoot) UnmarshalInventory(ctx context.Context, v any) error {
	f, err := obj.OpenFile(ctx, inventoryFile)
	if err != nil {
		return err
	}
	defer f.Close()
	bytes, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	return json.Unmarshal(bytes, v)
}

// OpenFile opens a file using a name relative to the object root's path
func (obj ObjectRoot) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if obj.Path != "." {
		// not using path.Join because it would collapse path elements,
		// potentially ignoring invalid values for obj.Path and name.
		name = obj.Path + "/" + name
	}
	return obj.FS.OpenFile(ctx, name)
}

// ReadDir reads a directory using a name relative to the object root's dir
func (obj ObjectRoot) ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error) {
	if obj.Path != "." {
		name = obj.Path + "/" + name
	}
	return obj.FS.ReadDir(ctx, name)
}

// checkState syncs the objec state if necessary and checks the
// an object declaration is present
func (obj *ObjectRoot) checkState(ctx context.Context) error {
	if obj.State == nil {
		if err := obj.SyncState(ctx); err != nil {
			return err
		}
	}
	if !obj.State.HasNamaste() {
		return ErrObjectNamasteNotExist
	}
	return nil
}

// ObjectRootState provides details of an OCFL object root based on the names of
// files and directories in the object's root. ParseObjectRootDir is typically
// used to create a new ObjectRootState from a slice of fs.DirEntry values.
type ObjectRootState struct {
	Spec        Spec           // the OCFL spec from the object's NAMASTE declaration file
	VersionDirs VNums          // version directories found in the object directory
	SidecarAlg  string         // digest algorithm used by the inventory sidecar file
	Invalid     []string       // non-conforming directory entries in the object root (max of 8)
	Flags       objectRootFlag // boolean attributes of the object root
}

type objectRootFlag uint8

// ParseObjectRootDir returns a new ObjectRootState based on contents of an
// object root directory.
func ParseObjectRootDir(entries []fs.DirEntry) *ObjectRootState {
	state := &ObjectRootState{}
	addInvalid := func(name string) {
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
				// invalid directory
				addInvalid(name)
			}
		case validFileType(e.Type()):
			switch {
			case name == inventoryFile:
				state.Flags |= HasInventory
			case strings.HasPrefix(name, sidecarPrefix):
				if state.HasSidecar() {
					// duplicate sidecar-like file
					addInvalid(name)
					break
				}
				state.SidecarAlg = strings.TrimPrefix(name, sidecarPrefix)
				state.Flags |= HasSidecar
			case strings.HasPrefix(name, objectDeclPrefix):
				if state.HasNamaste() {
					// duplicate namaste
					addInvalid(name)
					break
				}
				decl, err := ParseNamaste(name)
				if err != nil {
					addInvalid(name)
					break
				}
				state.Spec = decl.Version
				state.Flags |= HasNamaste
			default:
				// invalid file
				addInvalid(name)
			}
		default:
			// invalid mode type
			addInvalid(name)
		}
	}
	return state
}

// HasNamaste returns true if state's HasNamaste flag is set
func (state ObjectRootState) HasNamaste() bool {
	return state.Flags&HasNamaste > 0
}

// HasInventory returns true if state's HasInventory flag is set
func (state ObjectRootState) HasInventory() bool {
	return state.Flags&HasInventory > 0
}

// HasSidecar returns true if state's HasSidecar flag is set
func (state ObjectRootState) HasSidecar() bool {
	return state.Flags&HasSidecar > 0
}

// HasExtensions returns true if state's HasExtensions flag is set
func (state ObjectRootState) HasExtensions() bool {
	return state.Flags&HasExtensions > 0
}

// HasVersionDir returns true if the state's VersionDirs includes v
func (state ObjectRootState) HasVersionDir(v VNum) bool {
	return slices.Contains(state.VersionDirs, v)
}

// ObjectRootsFS is an FS with an optimized implementation of ObjectRoots
type ObjectRootsFS interface {
	FS
	// ObjectRoots searches root and its subdirectories for OCFL object declarations
	// and returns an iterator that yields each object root it finds. The
	// *ObjectRoot passed to yield is confirmed to have an object declaration, but
	// no other validation checks are made.
	ObjectRoots(ctx context.Context, dir string) ObjectRootSeq
}

// ObjectRootSeq is an iterator that yields ObjectRoot references; it is returned
// by ObjectRoots()
type ObjectRootSeq func(yield func(*ObjectRoot, error) bool)

// ObjectRoots searches dir in fsys (and its subdirectories) for OCFL object
// declarations and returns an iterator that yields each object root it finds.
// The *ObjectRoot passed back to ObjectRootSeq's yield function is confirmed to
// have an object declaration, but no other validation checks are made. If fsys
// is an ObjectRootsFS, its implementation of ObjectRoots is used.
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
		State: ParseObjectRootDir(entries),
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
