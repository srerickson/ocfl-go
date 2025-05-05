package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"

	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/internal/logical-fs"
)

// Object represents and OCFL Object, typically contained in a Root.
type Object struct {
	// object's storage backend. Must implement WriteFS to commit.
	fs ocflfs.FS
	// path in FS for object root directory
	path string
	// object's root inventory. May be nil if the object doesn't (yet) exist.
	//root inventory
	rootInventory *Inventory
	// the expected object ID
	expectID string
	// the object must exist: don't create a new object.
	mustExist bool
	// object's storage root
	root *Root
}

// NewObject returns an *Object for managing the OCFL object at path in fsys.
// The object doesn't need to exist when NewObject is called.
func NewObject(ctx context.Context, fsys ocflfs.FS, dir string, opts ...ObjectOption) (*Object, error) {
	if !fs.ValidPath(dir) {
		return nil, fmt.Errorf("invalid object path: %q: %w", dir, fs.ErrInvalid)
	}
	obj := newObject(fsys, dir, opts...)
	// read root inventory: we don't know what OCFL spec it uses.
	inv, err := ReadInventory(ctx, fsys, dir)
	if err != nil {
		// continue of err is ErrNotExist and !mustExist
		if !obj.mustExist && errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
		if err != nil {
			return nil, err
		}
	}
	if inv != nil {
		// check that inventory has expected object ID
		// if the expected object ID is known.
		if obj.expectID != "" && inv.ID != obj.expectID {
			err := fmt.Errorf("object has unexpected ID: %q; expected: %q", inv.ID, obj.expectID)
			return nil, err
		}
		obj.rootInventory = inv
		return obj, nil
	}
	// inventory doesn't exist: open as uninitialized object. The object
	// root directory must not exist or be an empty directory. the object's
	// inventory is nil.
	entries, err := ocflfs.ReadDir(ctx, fsys, dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("reading object root directory: %w", err)
		}
	}
	rootState := ParseObjectDir(entries)
	switch {
	case rootState.Empty():
		return obj, nil
	case rootState.HasNamaste():
		return nil, fmt.Errorf("incomplete OCFL object: %s: %w", inventoryBase, fs.ErrNotExist)
	default:
		return nil, fmt.Errorf("directory is not an OCFL object: %w", ErrObjectNamasteNotExist)
	}
}

// ContentDirectory return "content" or the value set in the root inventory.
func (obj Object) ContentDirectory() string {
	if obj.rootInventory != nil && obj.rootInventory.ContentDirectory != "" {
		return obj.rootInventory.ContentDirectory
	}
	return contentDir
}

// Commit creates a new object version based on values in commit.
func (obj *Object) Commit(ctx context.Context, commit *Commit) error {
	if _, isWriteFS := obj.FS().(ocflfs.WriteFS); !isWriteFS {
		return errors.New("object's backing file system doesn't support write operations")
	}
	// the OCFL implementation for the new object version
	var useOCFL ocfl
	switch {
	case commit.Spec.Empty():
		switch {
		case !obj.Exists():
			// new object and no ocfl version specified in commit
			useOCFL = defaultOCFL()
		default:
			// use existing object's ocfl version
			var err error
			useOCFL, err = getOCFL(obj.rootInventory.Type.Spec)
			if err != nil {
				err = fmt.Errorf("object's root inventory has errors: %w", err)
				return &CommitError{Err: err}
			}
		}
		commit.Spec = useOCFL.Spec()
	default:
		var err error
		useOCFL, err = getOCFL(commit.Spec)
		if err != nil {
			return &CommitError{Err: err}
		}
	}
	// set commit's object id if we have an expected id and commit ID isn't set
	if obj.expectID != "" && commit.ID != obj.expectID {
		if commit.ID != "" {
			err := fmt.Errorf("commit includes unexpected object ID: %s; expected: %q", commit.ID, obj.expectID)
			return &CommitError{Err: err}
		}
		commit.ID = obj.expectID
	}
	if err := useOCFL.Commit(ctx, obj, commit); err != nil {
		return err
	}
	return nil
}

// DigestAlgorithm returns sha512 unless sha256 is set in the root inventory.
func (obj Object) DigestAlgorithm() digest.Algorithm {
	if obj.rootInventory != nil && obj.rootInventory.DigestAlgorithm == digest.SHA256.ID() {
		return digest.SHA256
	}
	return digest.SHA512
}

// Exists returns true if the object has an existing version.
func (obj Object) Exists() bool {
	return obj.rootInventory != nil
}

// ExtensionNames returns the names of directories in the object's
// extensions directory. The ObjectRoot's State is initialized if it is
// nil. If the object root does not include an object declaration, an error
// is returned. If object root does not include an extensions directory both
// return values are nil.
func (obj Object) ExtensionNames(ctx context.Context) ([]string, error) {
	entries, err := ocflfs.ReadDir(ctx, obj.FS(), path.Join(obj.path, extensionsDir))
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

// FixityAlgorithms returns a slice of the keys from the root inventory's
// `fixity` block, or nil of the root inventory is not set.
func (obj Object) FixityAlgorithms() []string {
	if obj.rootInventory == nil {
		return nil
	}
	return slices.Collect(maps.Keys(obj.rootInventory.Fixity))
}

// FS returns the FS where object is stored.
func (obj *Object) FS() ocflfs.FS {
	return obj.fs
}

// GetFixity implement the FixitySource interface for Object, for use in Stage.
func (obj Object) GetFixity(dig string) digest.Set {
	if obj.rootInventory == nil {
		return nil
	}
	return obj.rootInventory.GetFixity(dig)
}

// GetContent implements ContentSource for Object, for use in Stage.
func (obj Object) GetContent(dig string) (ocflfs.FS, string) {
	if obj.rootInventory == nil {
		return nil, ""
	}
	paths := obj.rootInventory.Manifest[dig]
	if len(paths) < 1 {
		return nil, ""
	}
	return obj.fs, path.Join(obj.path, paths[0])
}

// Head returns the most recent version number. If obj has no root inventory, it
// returns the zero value.
func (obj Object) Head() VNum {
	if obj.rootInventory == nil {
		return VNum{}
	}
	return obj.rootInventory.Head
}

// ID returns obj's inventory ID if the obj exists (its inventory is not nil).
// If obj does not exist but was constructed with [Root.NewObject](), the ID
// used with [Root.NewObject]() is returned. Otherwise, it returns an empty
// string.
func (obj Object) ID() string {
	if obj.rootInventory != nil {
		return obj.rootInventory.ID
	}
	return obj.expectID
}

// Manifest returns a copy of the root inventory manifest. If the object has no
// root inventory (e.g., it doesn't yet exist), nil is returned.
func (obj Object) Manifest() DigestMap {
	if obj.rootInventory == nil {
		return nil
	}
	if obj.rootInventory.Manifest == nil {
		return DigestMap{}
	}
	return obj.rootInventory.Manifest.Clone()
}

// Path returns the Object's path relative to its FS.
func (obj *Object) Path() string {
	return obj.path
}

// Root returns the object's Root, if known. It is nil unless the *Object was
// created using [Root.NewObject]
func (o *Object) Root() *Root {
	return o.root
}

// Spec returns the OCFL spec number from the object's root inventory, or an
// empty Spec if the root inventory does not exist.
func (o *Object) Spec() Spec {
	if o.rootInventory == nil {
		return Spec("")
	}
	return o.rootInventory.Type.Spec
}

// Version returns a pointer to a copy of the InventoryVersion with the given
// number (1...HEAD) from the root inventory. For example, v == 1 refers to "v1"
// or "v001" version block. If v < 1, the most recent version is returned. If
// the version does not exist, nil is returned. The returned
func (obj Object) Version(v int) *ObjectVersion {
	if obj.rootInventory == nil {
		return nil
	}
	vnum := obj.rootInventory.Head
	if v > 0 {
		vnum = V(v, obj.rootInventory.Head.padding)
	}
	ver := obj.rootInventory.Versions[vnum]
	if ver == nil {
		return nil
	}
	return &ObjectVersion{
		vnum:    vnum,
		version: ver,
	}
}

// VersionFS returns an io/fs.FS representing the logical state for the version
// with the given number (1...HEAD). If v < 1, the most recent version is used.
func (obj *Object) VersionFS(ctx context.Context, v int) (fs.FS, error) {
	ver := obj.version(v)
	if ver == nil {
		return nil, errors.New("version not found")
	}
	// map logical names to content paths
	logicalNames := make(map[string]string, ver.State.NumPaths())
	for name, digest := range ver.State.Paths() {
		realNames := obj.rootInventory.Manifest[digest]
		if len(realNames) < 1 {
			err := errors.New("missing manifest entry for digest: " + digest)
			return nil, err
		}
		logicalNames[name] = path.Join(obj.path, realNames[0])
	}
	fsys := logical.NewLogicalFS(
		ctx,
		obj.fs,
		logicalNames,
		ver.Created,
	)
	return fsys, nil
}

// VersionStage returns a *Stage matching the content of the version with the
// given number (1...HEAD). If ther version does not exist, nil is returned.
func (obj *Object) VersionStage(v int) *Stage {
	ver := obj.version(v)
	if ver == nil {
		return nil
	}
	return &Stage{
		State:           ver.State,
		DigestAlgorithm: obj.DigestAlgorithm(),
		ContentSource:   obj,
		FixitySource:    obj,
	}
}

func (obj Object) version(v int) *InventoryVersion {
	if obj.rootInventory == nil {
		return nil
	}
	return obj.rootInventory.version(v)
}

// ValidateObject fully validates the OCFL Object at dir in fsys
func ValidateObject(ctx context.Context, fsys ocflfs.FS, dir string, opts ...ObjectValidationOption) *ObjectValidation {
	v := newObjectValidation(fsys, dir, opts...)
	if !fs.ValidPath(dir) {
		err := fmt.Errorf("invalid object path: %q: %w", dir, fs.ErrInvalid)
		v.AddFatal(err)
		return v
	}
	entries, err := ocflfs.ReadDir(ctx, fsys, dir)
	if err != nil {
		v.AddFatal(err)
		return v
	}
	state := ParseObjectDir(entries)
	impl, err := getOCFL(state.Spec)
	if err != nil {
		// unknown OCFL version
		v.AddFatal(err)
		return v
	}
	if err := impl.ValidateObjectRoot(ctx, v, state); err != nil {
		return v
	}
	// validate versions using previous specs
	versionOCFL := lowestOCFL()
	var prevInv *Inventory
	for _, vnum := range state.VersionDirs.Head().Lineage() {
		versionDir := path.Join(dir, vnum.String())
		versionInv, err := ReadInventory(ctx, fsys, versionDir)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			v.AddFatal(fmt.Errorf("reading %s/inventory.json: %w", vnum, err))
			continue
		}
		if versionInv != nil {
			versionOCFL = mustGetOCFL(versionInv.Type.Spec)
		}
		versionOCFL.ValidateObjectVersion(ctx, v, vnum, versionInv, prevInv)
		prevInv = versionInv
	}
	impl.ValidateObjectContent(ctx, v)
	return v
}

// create a new *Object with required feilds and apply options
func newObject(fsys ocflfs.FS, dir string, opts ...ObjectOption) *Object {
	obj := &Object{fs: fsys, path: dir}
	for _, optFn := range opts {
		optFn(obj)
	}
	return obj
}

// ObjectOptions are used to configure the behavior of NewObject()
type ObjectOption func(*Object)

// ObjectMustExists requires the object to exist
func ObjectMustExist() ObjectOption {
	return func(o *Object) {
		o.mustExist = true
	}
}

// objectExpectedID is an ObjectOption to set the expected ID (i.e., from )
func objectExpectedID(id string) ObjectOption {
	return func(o *Object) {
		o.expectID = id
	}
}

// objectWithRoot is an ObjectOption that sets the object's storage root
func objectWithRoot(root *Root) ObjectOption {
	return func(o *Object) {
		o.root = root
	}
}

// ObjectVersion is used to access version information from an object's root
// inventory.
type ObjectVersion struct {
	vnum    VNum
	version *InventoryVersion
}

// Message returns the version's message
func (o ObjectVersion) Message() string { return o.version.Message }

// State returns a copy of the version's state
func (o ObjectVersion) State() DigestMap { return o.version.State.Clone() }

// User returns the version's user information, which may be nil
func (o ObjectVersion) User() *User {
	var user *User
	if o.version.User != nil {
		user = &User{}
		*user = *o.version.User
	}
	return user
}

// VNum returns o's version number
func (o ObjectVersion) VNum() VNum { return o.vnum }
