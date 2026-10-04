package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"time"

	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/internal/logical-fs"
	"github.com/srerickson/ocfl-go/validation/code"
)

var ErrNoObjectID = errors.New("object does not exist: an explicit ID is required but was not provided")

// Object is a read-only view of an OCFL Object, typically part of a [Root],
// as described by one inventory. An Object never changes: to create a new
// version, use [Object.NewUpdate] and apply the update with
// [ObjectUpdate.Apply], which returns a new *Object. The previous *Object
// remains a valid but out-of-date view.
type Object struct {
	// object's storage backend.
	fs ocflfs.FS
	// path in FS for object root directory
	path string
	// object's root inventory (unless object is initialized with an explicit
	// inventory!). May be nil if the object hasn't been saved yet.
	inventory *StoredInventory
	// object's storage root
	root *Root
	// expected object ID
	requiredID string
}

// NewObject returns an *Object for reading the OCFL object at directory dir in
// fsys. The object doesn't need to exist when NewObject is called if its ID is
// given with [ObjectWithID]: the returned *Object can be used to create the
// object with [Object.NewUpdate].
func NewObject(ctx context.Context, fsys ocflfs.FS, dir string, opts ...ObjectOption) (*Object, error) {
	if !fs.ValidPath(dir) {
		return nil, fmt.Errorf("invalid object path: %q: %w", dir, fs.ErrInvalid)
	}
	obj, config := newObjectAndConfig(fsys, dir, opts...)
	if obj.inventory == nil {
		if err := obj.sync(ctx); err != nil {
			if config.mustExist || !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
		}
	}
	if obj.inventory != nil {
		inv := obj.inventory
		if obj.requiredID != "" && inv.ID != obj.requiredID {
			err := fmt.Errorf("object has unexpected ID: %q; expected: %q", inv.ID, obj.requiredID)
			return nil, err
		}
		if !config.skipRootSidecarValidation {
			if err := inv.ValidateSidecar(ctx, fsys, dir); err != nil {
				var digestErr *digest.DigestError
				if errors.Is(err, fs.ErrNotExist) || errors.As(err, &digestErr) {
					return nil, fmt.Errorf("%w: %w", ErrObjectIncomplete, err)
				}
				return nil, err
			}
		}
		return obj, nil
	}
	if obj.requiredID == "" {
		return nil, ErrNoObjectID
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
		return nil, fmt.Errorf("%w: missing %s: %w", ErrObjectIncomplete, inventoryBase, fs.ErrNotExist)
	default:
		return nil, errors.New("directory is not empty: non-conforming contents")
	}
}

// ContentDirectory return "content" or the value set in the root inventory.
func (obj Object) ContentDirectory() string {
	if obj.inventory != nil && obj.inventory.ContentDirectory != "" {
		return obj.inventory.ContentDirectory
	}
	return contentDir
}

// NewUpdate returns a new draft *[ObjectUpdate] for the object's next version.
// The update starts from the head version state and the object's digest
// algorithm (sha512 for an object that doesn't exist yet). NewUpdate does no
// I/O: if the object has changed in storage since obj was created, applying
// the update fails with an error wrapping [ErrUpdateConflict].
func (obj *Object) NewUpdate() *ObjectUpdate {
	var rootSpec Spec
	if obj.root != nil {
		rootSpec = obj.root.Spec()
	}
	u, err := newObjectUpdate(obj.ID(), obj.inventory, rootSpec, nil)
	if err != nil {
		// obj's inventory is valid and, if obj doesn't exist, its ID is set.
		panic(fmt.Sprintf("ocfl: Object.NewUpdate: %v", err))
	}
	return u
}

// DigestAlgorithm returns sha512 unless sha256 is set in the root inventory.
func (obj Object) DigestAlgorithm() digest.Algorithm {
	if obj.inventory != nil && obj.inventory.DigestAlgorithm == digest.SHA256.ID() {
		return digest.SHA256
	}
	return digest.SHA512
}

// Exists returns true if the object has an existing version.
func (obj Object) Exists() bool {
	return obj.inventory != nil
}

// ExtensionNames returns the names of directories in the object's
// extensions directory.
func (obj Object) ExtensionNames(ctx context.Context) ([]string, error) {
	entries, err := ocflfs.ReadDir(ctx, obj.FS(), path.Join(obj.path, extensionsDir))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, err
}

// FixityAlgorithms returns a slice of the keys from the `fixity` block of obj's
// return inventory. If obj does not have an inventory (i.e., because one has not
// been created yet), it returns nil.
func (obj Object) FixityAlgorithms() []string {
	if obj.inventory == nil {
		return nil
	}
	return slices.Collect(maps.Keys(obj.inventory.Fixity))
}

// FS returns the FS where object is stored.
func (obj Object) FS() ocflfs.FS {
	return obj.fs
}

// Head returns the most recent version number. If obj has no root inventory, it
// returns the zero value.
func (obj Object) Head() VNum {
	if obj.inventory == nil {
		return VNum{}
	}
	return obj.inventory.Head
}

// InventoryDigest returns the digest of the object's root inventory using the
// declarate digest algorithm. It is the expected content of the root
// inventory's sidecar file.
func (obj Object) InventoryDigest() string {
	if obj.inventory == nil {
		return ""
	}
	return obj.inventory.Digest()
}

// ID returns obj's inventory ID if the obj exists (its inventory is not nil).
// If obj does not exist but was constructed with [Root.NewObject] or with the
// [ObjectWithID] option, the ID given as an argument is returned. Otherwise, it
// returns an empty string.
func (obj Object) ID() string {
	if obj.inventory != nil {
		return obj.inventory.ID
	}
	return obj.requiredID
}

// Manifest returns a copy of the root inventory manifest. If the object has no
// root inventory (e.g., it doesn't yet exist), nil is returned.
func (obj Object) Manifest() DigestMap {
	if obj.inventory == nil {
		return nil
	}
	if obj.inventory.Manifest == nil {
		return DigestMap{}
	}
	return obj.inventory.Manifest.Clone()
}

// Path returns the Object's path relative to its FS.
func (obj Object) Path() string {
	return obj.path
}

// Root returns the object's Root, if known. It is nil unless the *Object was
// created using [Root.NewObject]
func (o Object) Root() *Root {
	return o.root
}

// Spec returns the OCFL spec number from the object's root inventory, or an
// empty Spec if the root inventory does not exist.
func (o Object) Spec() Spec {
	if o.inventory == nil {
		return Spec("")
	}
	return o.inventory.Type.Spec
}

// Version returns an *ObjectVersion that can be used to access details for the
// version with the given number (1...HEAD) from the root inventory. For
// example, v == 1 refers to "v1" or "v001" version block. If v < 1, the most
// recent version is returned. If the version does not exist, nil is returned.
func (obj Object) Version(v int) *ObjectVersion {
	if obj.inventory == nil {
		return nil
	}
	vnum := obj.inventory.Head
	if v > 0 {
		vnum = V(v, obj.inventory.Head.padding)
	}
	ver := obj.inventory.Versions[vnum]
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
		realNames := obj.inventory.Manifest[digest]
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

func (obj Object) version(v int) *InventoryVersion {
	if obj.inventory == nil {
		return nil
	}
	return obj.inventory.version(v)
}

// sync re-reads the object's root inventory, updating obj's internal state
func (obj *Object) sync(ctx context.Context) error {
	inv, err := ReadInventory(ctx, obj.fs, obj.path)
	if err != nil {
		return fmt.Errorf("%q inventory: %w", obj.ID(), err)
	}
	obj.inventory = inv
	return nil
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
	spec := state.Spec
	if spec == "" {
		// No Namaste file found, use default OCFL version for validation
		// The missing Namaste will be reported as E003 by ValidateObjectRoot
		spec = Spec1_1
	}
	impl, err := getOCFL(spec)
	if err != nil {
		// unknown OCFL version
		v.AddFatal(err)
		return v
	}
	// E081: if the object is validated as part of a storage root, its declared
	// spec must not be newer than the root's spec.
	if root := v.obj.root; root != nil && !root.Spec().Empty() && !state.Spec.Empty() {
		if rootSpec := root.Spec(); state.Spec.Cmp(rootSpec) > 0 {
			err := fmt.Errorf("object declares OCFL v%s but its storage root declares OCFL v%s", state.Spec, rootSpec)
			v.AddFatal(verr(err, code.E081(string(rootSpec))))
		}
	}
	if err := impl.ValidateObjectRoot(ctx, v, state); err != nil {
		return v
	}
	// validate versions using previous specs
	versionOCFL := lowestOCFL()
	var prevInv *StoredInventory
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
		_ = versionOCFL.ValidateObjectVersion(ctx, v, vnum, versionInv, prevInv)
		prevInv = versionInv
	}
	_ = impl.ValidateObjectContent(ctx, v)
	return v
}

// ObjectOptions are used to configure the behavior of NewObject()
type ObjectOption func(*newObjectConfig)

// ObjectMustExists is an ObjectOption used to indicate that the initialized
// object instance must be an existing OCFL object.
func ObjectMustExist() ObjectOption {
	return func(o *newObjectConfig) {
		o.mustExist = true
	}
}

// ObjectSkipRootSidecarValidation is used to skip validating the inventory.json
// digest with the root inventory sidecar file during initialization.
func ObjectSkipRootSidecarValidation() ObjectOption {
	return func(o *newObjectConfig) {
		o.skipRootSidecarValidation = true
	}
}

// ObjectWithID is an ObjectOption used to set an explict object ID
// in contexts where the object ID is not known or must match a certain
// value.
func ObjectWithID(id string) ObjectOption {
	return func(o *newObjectConfig) {
		o.requiredID = id
	}
}

// ObjectWithInventory is used to initialize an *Object using an existing
// *StoredInventory value. It can be used with an inventory cache to minimize
// requests to the object's storage backed. Unless it is combined with the
// [ObjectSkipRootSidecarValidation] option, inv's digest will be validated
// against the root inventory sidecar file.
func ObjectWithInventory(inv *StoredInventory) ObjectOption {
	return func(o *newObjectConfig) {
		o.inv = inv
	}
}

// objectWithRoot is an ObjectOption that sets the object's storage root.
// It's only meant to be used in Root methods.
func objectWithRoot(root *Root) ObjectOption {
	return func(o *newObjectConfig) {
		if o.root == nil {
			o.root = root
		}
	}
}

type newObjectConfig struct {
	// object's expected id
	requiredID string
	// the object must exist: don't create a new object.
	mustExist bool
	// during initialization, don't check that the inventory's digest matches
	// the contents of the inventory sidecar file.
	skipRootSidecarValidation bool
	// object's storage root
	root *Root
	// storedInventory is an explicit inventory to open the object with
	inv *StoredInventory
}

// create a new *Object with required feilds and apply options
func newObjectAndConfig(fsys ocflfs.FS, dir string, opts ...ObjectOption) (*Object, *newObjectConfig) {
	var config newObjectConfig
	for _, optFn := range opts {
		optFn(&config)
	}
	return &Object{
		fs:         fsys,
		path:       dir,
		root:       config.root,
		requiredID: config.requiredID,
		inventory:  config.inv,
	}, &config
}

// ObjectVersion is used to access version information from an object's root
// inventory.
type ObjectVersion struct {
	vnum    VNum
	version *InventoryVersion
}

// Created returns the version's created timestamp
func (o ObjectVersion) Created() time.Time { return o.version.Created }

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
