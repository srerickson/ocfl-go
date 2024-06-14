package ocfl

import (
	"context"
	"encoding/json"
	"errors"
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

// ObjectRoot represents an OCFL object root directory.
type ObjectRoot struct {
	// FS is the FS for accessing the object's contents.
	FS FS
	// Path is the path in the FS for the object root directory.
	Path string
	// State provides details about an existing object root as determined by
	// reading the contents of the directory with ReadRoot(). State may be nil
	// if the object root has not be read or if an error occured while reading
	// it.
	State *ObjectRootState

	// stateErr is the error from ReadRoot() used to set State.
	stateErr error
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
	if err := obj.mustHaveNamaste(ctx); err != nil {
		return nil, err
	}
	return obj, nil
}

// ValidateNamaste reads and validates the contents of the OCFL object
// declaration in the object root. The ObjectRoot's State is initialized if it
// is nil.
func (obj *ObjectRoot) ValidateNamaste(ctx context.Context) error {
	if err := obj.mustHaveNamaste(ctx); err != nil {
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
	// state needs to be checked in order to differentiate between the case of
	// of the object root not an existing (an error) and the extensions
	// directory not existing (not an error).
	if err := obj.mustHaveNamaste(ctx); err != nil {
		return nil, err
	}
	if !obj.State.HasExtensions() {
		return nil, nil
	}
	entries, err := obj.ReadDir(ctx, ExtensionsDir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
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

// UnmarshalInventory unmarshals the inventory.json file in the object root's
// sub-directory, dir, into the value pointed to by v. For example, set dir to
// `v1` to unmarshall the object's v1 inventory. Set dir to `.` to unmarshal the
// root inventory.
func (obj ObjectRoot) UnmarshalInventory(ctx context.Context, dir string, v any) (err error) {
	name := inventoryFile
	if dir != `.` {
		name = dir + "/" + name
	}
	f, err := obj.OpenFile(ctx, name)
	if err != nil {
		return
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, f.Close())
		}
	}()
	bytes, err := io.ReadAll(f)
	if err != nil {
		return
	}
	err = json.Unmarshal(bytes, v)
	return
}

// OpenFile opens a file using a name relative to the object root's path
func (obj *ObjectRoot) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if obj.Path != "." {
		// using path.Join might hide potentially invalid values for
		// obj.Path or name.
		name = obj.Path + "/" + name
	}
	return obj.FS.OpenFile(ctx, name)
}

// ReadDir reads a directory using a name relative to the object root's dir. If name
// is ".", obj's State value is updated using the returned values.
func (obj *ObjectRoot) ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error) {
	if name == "." {
		// we're reading the object root, so update state
		var entries []fs.DirEntry
		entries, obj.stateErr = obj.FS.ReadDir(ctx, obj.Path)
		if obj.stateErr != nil {
			return nil, obj.stateErr
		}
		obj.State = ParseObjectRootDir(entries)
		return entries, nil
	}
	if obj.Path != "." {
		name = obj.Path + "/" + name
	}
	return obj.FS.ReadDir(ctx, name)
}

// ReadRoot reads the contents of the object root directory and updates
// obj.State.
func (obj *ObjectRoot) ReadRoot(ctx context.Context) error {
	_, err := obj.ReadDir(ctx, ".")
	return err
}

// Exists returns two bools: the first indicates if the existence status of the
// object's root directory is known; the second indicates the existence status.
// The second value should only be used if the first is true.
func (obj *ObjectRoot) Exists(ctx context.Context) (bool, error) {
	if obj.State == nil && obj.stateErr == nil {
		// error is retained in stateErr
		obj.ReadRoot(ctx)
	}
	if obj.stateErr != nil {
		if errors.Is(obj.stateErr, fs.ErrNotExist) {
			return false, nil
		}
		return false, obj.stateErr
	}
	if obj.State == nil {
		panic("ReadRoot didn't set objcet state as expected")
	}
	return true, nil
}

// mustHaveNamaste syncs the objec state if necessary and checks that
// an object declaration is present
func (obj *ObjectRoot) mustHaveNamaste(ctx context.Context) error {
	if obj.State == nil {
		if err := obj.ReadRoot(ctx); err != nil {
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

// Empty returns true if the object root directory is empty
func (state ObjectRootState) Empty() bool {
	return state.Flags == 0 && len(state.VersionDirs) == 0 && len(state.Invalid) == 0
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
