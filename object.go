package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"path"
	"slices"
	"time"

	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/internal/logical-fs"
	"github.com/srerickson/ocfl-go/logging"
)

// Object represents and OCFL Object, typically contained in a Root.
type Object struct {
	// object's storage backend. Must implement WriteFS to update.
	fs ocflfs.FS
	// path in FS for object root directory
	path string
	// object's root inventory. May be nil if the object doesn't (yet) exist.
	//root inventory
	rootInventory *StoredInventory
	//rootInventoryBytes is raw root inventory data
	rootInventoryBytes []byte
	// object's storage root
	root *Root
	// expected object ID
	requiredID string
}

// NewObject returns an *Object for managing the OCFL object at path in fsys.
// The object doesn't need to exist when NewObject is called.
func NewObject(ctx context.Context, fsys ocflfs.FS, dir string, opts ...ObjectOption) (*Object, error) {
	if !fs.ValidPath(dir) {
		return nil, fmt.Errorf("invalid object path: %q: %w", dir, fs.ErrInvalid)
	}
	obj, config := newObjectAndConfig(fsys, dir, opts...)
	if err := obj.SyncInventory(ctx); err != nil {
		if !config.mustExist && errors.Is(err, fs.ErrNotExist) {
			err = nil //it's ok that the inventory doesn't exist
		}
		if err != nil {
			return nil, err
		}
	}
	if obj.rootInventory != nil {
		inv := obj.rootInventory
		if obj.requiredID != "" && inv.ID != obj.requiredID {
			err := fmt.Errorf("object has unexpected ID: %q; expected: %q", inv.ID, obj.requiredID)
			return nil, err
		}
		if !config.skipSidecarValidation {
			err := inv.ValidateSidecar(ctx, fsys, dir)
			if err != nil {
				return nil, err
			}
		}
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

// NewInventoryBuidler returns a new *InventoryBuilder that can be used to
// generate obj's next root inventory.
func (obj *Object) NewInventoryBuilder() *InventoryBuilder {
	var base *Inventory
	if obj.rootInventory != nil {
		base = &obj.rootInventory.Inventory
	}
	return NewInventoryBuilder(base).ID(obj.ID())
}

// NewUpdatePlan builds a new object inventory using stage's state and returns
// an *UpdatePlan that can be used to apply the changes. It does not apply the
// update plan.
func (obj *Object) NewUpdatePlan(stage *Stage, msg string, user User, opts ...ObjectUpdateOption) (*UpdatePlan, error) {
	updateOpts := newObjectUpdateOptions(opts...)
	newInv, err := obj.NewInventoryBuilder().
		FixitySource(stage).
		ContentPathFunc(updateOpts.ContentPathFunc).
		Spec(updateOpts.Spec).
		AddVersion(
			stage.State,
			stage.DigestAlgorithm,
			updateOpts.Created,
			msg,
			&user,
		).Finalize()
	if err != nil {
		return nil, fmt.Errorf("building new inventory for update: %w", err)
	}
	currentInv := obj.rootInventory
	if !updateOpts.AllowUnchanged && currentInv != nil {
		lastV := currentInv.Versions[currentInv.Head]
		if lastV != nil && lastV.State.Eq(stage.State) {
			return nil, errors.New("update has unchanged version state")
		}
	}
	plan, err := NewUpdatePlan(obj, newInv, stage.ContentSource)
	if err != nil {
		return nil, fmt.Errorf("in object update plan: %w", err)
	}
	plan.SetGoLimit(updateOpts.goLimit)
	plan.SetLogger(updateOpts.logger)
	return plan, nil
}

// Update creates an update plan using obj.NewUpdatePlan and applies it.
func (obj *Object) Update(ctx context.Context, stage *Stage, msg string, user User, opts ...ObjectUpdateOption) error {
	plan, err := obj.NewUpdatePlan(stage, msg, user, opts...)
	if err != nil {
		return err
	}
	updatedObj, err := plan.Apply(ctx)
	if err != nil {
		return err
	}
	*obj = *updatedObj
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
func (obj Object) FS() ocflfs.FS {
	return obj.fs
}

// GetFixity implements the FixitySource interface for Object, for use in Stage.
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

// InventoryDigest returns the digest of the object's root inventory using the
// declarate digest algorithm. It is the expected content of the root
// inventory's sidecar file.
func (obj Object) InventoryDigest() string {
	if obj.rootInventory == nil {
		return ""
	}
	return obj.rootInventory.Digest()
}

// ID returns obj's inventory ID if the obj exists (its inventory is not nil).
// If obj does not exist but was constructed with [Root.NewObject](), the ID
// used with [Root.NewObject]() is returned. Otherwise, it returns an empty
// string.
func (obj Object) ID() string {
	if obj.rootInventory != nil {
		return obj.rootInventory.ID
	}
	return obj.requiredID
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
	if o.rootInventory == nil {
		return Spec("")
	}
	return o.rootInventory.Type.Spec
}

// Version returns an *ObjectVersion that can be used to access details for the
// version with the given number (1...HEAD) from the root inventory. For
// example, v == 1 refers to "v1" or "v001" version block. If v < 1, the most
// recent version is returned. If the version does not exist, nil is returned.
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

// SyncInventory re-reads the object's root inventory, updating obj's internal state
func (obj *Object) SyncInventory(ctx context.Context) error {
	byts, err := ocflfs.ReadAll(ctx, obj.fs, path.Join(obj.path, inventoryBase))
	if err != nil {
		return fmt.Errorf("reading %q inventory: %w", obj.ID(), err)
	}
	inv, err := newStoredInventory(byts)
	if err != nil {
		return fmt.Errorf("in %q inventory: %w", obj.ID(), err)
	}
	obj.rootInventory = inv
	obj.rootInventoryBytes = byts
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

// ObjectSkipSidecarValidation is used to skip validating the inventory.json
// digest with the inventory sidecar file during initialization.
func ObjectSkipSidecarValidation() ObjectOption {
	return func(o *newObjectConfig) {
		o.skipSidecarValidation = true
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

// objectWithRoot is an ObjectOption that sets the object's storage root
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
	skipSidecarValidation bool
	// object's storage root
	root *Root
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
	}, &config
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

type objectUpdateOptions struct {
	Created         time.Time // time.Now is used, if not set
	Spec            Spec      // OCFL specification version for the new object version
	NewHEAD         int       // enforces new object version number
	AllowUnchanged  bool
	ContentPathFunc func(oldPaths []string) (newPaths []string)

	logger  *slog.Logger
	goLimit int
}

func newObjectUpdateOptions(opts ...ObjectUpdateOption) *objectUpdateOptions {
	combinedOpts := &objectUpdateOptions{
		logger: logging.DisabledLogger(),
	}
	for _, o := range opts {
		o(combinedOpts)
	}
	if combinedOpts.Created.IsZero() {
		combinedOpts.Created = time.Now()
	}
	return combinedOpts
}

// ObjectUpdateOptions are optional function arguments used to configure
// new an Object's UpdatePlan.
type ObjectUpdateOption func(*objectUpdateOptions)

// UpdateWithVersionCreated sets the 'created' timestamp for the object version
// created with the update.
func UpdateWithVersionCreated(t time.Time) ObjectUpdateOption {
	return func(o *objectUpdateOptions) {
		o.Created = t
	}
}

// UpdateWithOCFLSpec sets the OCFL specification for the new object version
func UpdateWithOCFLSpec(s Spec) ObjectUpdateOption {
	return func(o *objectUpdateOptions) {
		o.Spec = s
	}
}

// UpdateWithNewHead is used to enforce the expected version number (without
// padding) for the version created with the update. Without this, the
// new version increments the existing version number, whatever it may be.
func UpdateWithNewHead(v int) ObjectUpdateOption {
	return func(o *objectUpdateOptions) {
		o.NewHEAD = v
	}
}

// UpdateWithUnchangedVersionState is used to allow updates that don't change
// the version state. Without this, updates with the same state will result
// in an error.
func UpdateWithUnchangedVersionState() ObjectUpdateOption {
	return func(o *objectUpdateOptions) {
		o.AllowUnchanged = true
	}
}

// UpdateWithContentPathFunc is used to set a function for enforcing
// naming conventiosn for content paths.
func UpdateWithContentPathFunc(mutate PathMutation) ObjectUpdateOption {
	return func(o *objectUpdateOptions) {
		o.ContentPathFunc = mutate
	}
}

// UpdateWithLogger sets the logger using when applying the UpdatePlan
func UpdateWithLogger(logger *slog.Logger) ObjectUpdateOption {
	return func(o *objectUpdateOptions) {
		o.logger = nil
	}
}

// UpdateWithGoLimit sets the number of goroutines used to run
// concurrent steps in the UpdatePlan
func UpdateWithGoLimit(gos int) ObjectUpdateOption {
	return func(o *objectUpdateOptions) {
		o.goLimit = gos
	}
}
