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
)

var ErrObjectReadOnly = errors.New("object is read-only")
var ErrNoObjectID = errors.New("object does not exist: an explicit ID is required but was not provided")

// Object represents and OCFL Object, typically part of a [Root].
type Object struct {
	// object's storage backend. Must implement WriteFS to update.
	fs ocflfs.FS
	// path in FS for object root directory
	path string
	// object's root inventor (unless object is initialized with an explicit
	// inventory!). May be nil if the object hasn't been saved yet.
	inventory *StoredInventory
	// inventory has been been validated as the root inventory
	// either by reading it directly or by comparing the digest
	// of a given inventory to the inventory sidecar.
	inventoryIsRoot bool
	// object's storage root
	root *Root
	// expected object ID
	requiredID string
}

// NewObject returns an *Object for managing the OCFL object at directory dir in
// fsys. The object doesn't need to exist when NewObject is called.
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
			err := inv.ValidateSidecar(ctx, fsys, dir)
			if err != nil {
				return nil, err
			}
			obj.inventoryIsRoot = true
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
		return nil, fmt.Errorf("incomplete OCFL object: %s: %w", inventoryBase, fs.ErrNotExist)
	default:
		return nil, errors.New("directory is not empty: non-conforming contents")
	}
}

// ApplyPlan executes all activities in the plan and updates the object
func (obj *Object) ApplyPlan(ctx context.Context, plan *UpdatePlan, src ContentSource) error {
	if plan.ObjectID != obj.ID() {
		return fmt.Errorf("plan is for object %q but applying to %q", plan.ObjectID, obj.ID())
	}
	currentVersion := obj.Head()
	if currentVersion != plan.BaseVersion {
		return fmt.Errorf("plan base version %s does not match object's current version %s", plan.BaseVersion, currentVersion)
	}
	for _, activity := range plan.Activities {
		if activity.Status == ActivityStatusCompleted || activity.Status == ActivityStatusSkipped {
			continue // Already done
		}
		result, err := activity.Execute(ctx, obj.fs, obj.path, src)
		if err != nil {
			return fmt.Errorf("activity %q failed: %w", activity.ID, err)
		}
		if !result.Success {
			return fmt.Errorf("activity %q failed: %s", activity.ID, result.ErrorMessage)
		}
	}

	// Update object's inventory
	invBytes, invDigest, err := plan.newInventory.marshal()
	if err != nil {
		return fmt.Errorf("marshaling inventory: %w", err)
	}
	obj.inventory = &StoredInventory{
		Inventory: *plan.newInventory,
		digest:    invDigest,
		bytes:     invBytes,
	}
	obj.inventoryIsRoot = true
	return nil
}

// ContentDirectory return "content" or the value set in the root inventory.
func (obj Object) ContentDirectory() string {
	if obj.inventory != nil && obj.inventory.ContentDirectory != "" {
		return obj.inventory.ContentDirectory
	}
	return contentDir
}

// CreatePlan creates an update plan with an explicit timestamp without executing it.
// This is the primary method for creating plans deterministically.
//
// The timestamp parameter is required for deterministic execution in workflow engines
// where time.Now() cannot be used (for replay safety). In framework contexts:
// - Temporal workflows: use workflow.Now(ctx)
// - Durable Task Framework: use ctx.CurrentTimeUtc()
// - Regular Go code: use time.Now()
//
// Use ApplyPlan() to execute the plan and update the object.
func (obj *Object) CreatePlan(stage *Stage, timestamp time.Time, message string, user *User) (*UpdatePlan, error) {
	return obj.NewUpdatePlanBuilder(stage).
		WithTimestamp(timestamp).
		WithMessage(message).
		WithUser(user).
		Build()
}

// InventoryBuilder returns an *InventoryBuilder that can be used to generate
// new inventory's for the object.
func (obj *Object) InventoryBuilder() *InventoryBuilder {
	var base *Inventory
	if obj.inventory != nil {
		base = &obj.inventory.Inventory
	}
	return NewInventoryBuilder(base).ID(obj.ID())
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

// GetFixity implements the [FixitySource] interface for Object, for use in a [Stage].
func (obj Object) GetFixity(dig string) digest.Set {
	if obj.inventory == nil {
		return nil
	}
	return obj.inventory.GetFixity(dig)
}

// GetContent implements [ContentSource] for Object, for use in a [Stage].
func (obj Object) GetContent(dig string) (ocflfs.FS, string) {
	if obj.inventory == nil {
		return nil, ""
	}
	paths := obj.inventory.Manifest[dig]
	if len(paths) < 1 {
		return nil, ""
	}
	return obj.fs, path.Join(obj.path, paths[0])
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

// ReadOnly returns an error if obj does not support updates. Updates may be
// prohibited if obj's storage backend does not support writes or if it was
// initialized using an explicit inventory (using [ObjectWithInventory]) and
// without root sidecar validation (using [ObjectSkipRootSidecarValidation]).
func (obj Object) ReadOnly() error {
	if obj.inventory != nil && !obj.inventoryIsRoot {
		// if obj's inventory
		return fmt.Errorf("%w: initialized without latest inventory state", ErrObjectReadOnly)
	}
	if _, ok := obj.fs.(ocflfs.WriteFS); !ok {
		return fmt.Errorf("%w: storage backend does not support writes", ErrObjectReadOnly)
	}
	return nil
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

// Update creates a new version of the object using the provided stage.
// This is the main entry point for object updates.
//
// The update process:
// 1. Creates an update plan with activities
// 2. Executes all activities
// 3. Updates the object's inventory
//
// For durable execution frameworks, use NewUpdatePlanBuilder() instead
// to get explicit control over the plan creation and execution.
func (obj *Object) Update(ctx context.Context, stage *Stage, message string, user *User) error {
	// Check if object is read-only
	if err := obj.ReadOnly(); err != nil {
		return err
	}

	// Create plan
	plan, err := obj.NewUpdatePlanBuilder(stage).
		WithTimestamp(time.Now()).
		WithMessage(message).
		WithUser(user).
		Build()
	if err != nil {
		return fmt.Errorf("creating update plan: %w", err)
	}

	// Apply plan
	if err := obj.ApplyPlan(ctx, plan, stage.ContentSource); err != nil {
		return fmt.Errorf("applying update plan: %w", err)
	}

	return nil
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
	obj.inventoryIsRoot = true
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
		versionOCFL.ValidateObjectVersion(ctx, v, vnum, versionInv, prevInv)
		prevInv = versionInv
	}
	impl.ValidateObjectContent(ctx, v)
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
