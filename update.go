package ocfl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"path"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/logging"
	"golang.org/x/sync/errgroup"
)

var (
	// ErrFinalized is returned when an [ObjectUpdate] that has been finalized
	// is edited or finalized again.
	ErrFinalized = errors.New("object update is finalized")

	// ErrNotFinalized is returned when an [ObjectUpdate] that hasn't been
	// finalized is applied or reverted.
	ErrNotFinalized = errors.New("object update is not finalized")

	// ErrObjectIncomplete is returned when an update can't be started because
	// the object in storage is half-written, for example because an earlier
	// update was interrupted. The interrupted update can be resumed or
	// reverted with [ObjectUpdate.Apply] or [ObjectUpdate.Revert].
	ErrObjectIncomplete = errors.New("incomplete OCFL object: an update may have been interrupted")

	// ErrUpdateConflict is returned by [ObjectUpdate.Apply] and
	// [ObjectUpdate.Revert] when the object in storage matches neither the
	// update's base inventory nor its new inventory: the object was changed
	// by something else.
	ErrUpdateConflict = errors.New("object in storage has changed since the update was started")

	// ErrRevertUpdate is returned by [ObjectUpdate.Revert] when the update has
	// already been committed to storage.
	ErrRevertUpdate = errors.New("the update has completed and cannot be reverted")

	// ErrMissingContent is returned when an update can't be applied because
	// the [ContentSource] doesn't provide content for one or more digests
	// that are new in the update.
	ErrMissingContent = errors.New("content source is missing required content")

	// ErrUnexpectedHead is returned by [ObjectUpdate.Finalize] when the
	// version number of the new object version does not match the number
	// expected by [UpdateWithExpectedHead].
	ErrUnexpectedHead = errors.New("unexpected object version number")

	// ErrObjectSpecExceedsRoot is returned when an update would give an object
	// an OCFL specification version that is newer than the specification
	// version of the storage root that contains it (OCFL E081).
	ErrObjectSpecExceedsRoot = errors.New("object's OCFL spec cannot be newer than the storage root's OCFL spec")
)

// maximum number of missing digests listed in an ErrMissingContent error message
const maxMissingContentListed = 5

// VersionInfo is the metadata settled by [ObjectUpdate.Finalize] for the new
// version.
type VersionInfo struct {
	Message string
	User    User
	// Created is truncated to the second, as it is written to the inventory.
	Created time.Time
	Spec    Spec
}

// ObjectUpdate is a pending version of an OCFL object. It has two phases. As a
// draft, it holds the new version state and fixity for new content, which are
// changed with [ObjectUpdate.Add], [ObjectUpdate.Remove],
// [ObjectUpdate.Rename], and [ObjectUpdate.Clear]. [ObjectUpdate.Finalize]
// settles the version metadata and builds the new inventory, after which the
// update is read-only and can be applied to storage with [ObjectUpdate.Apply]
// or, if interrupted, resumed with Apply or undone with [ObjectUpdate.Revert].
//
// An ObjectUpdate is serializable as JSON in both phases. The saved form
// includes the object's inventory at the time the update was started (the
// base inventory), so nothing needs to be read from storage to finalize it. A
// finalized update should be saved before it is applied: if the process is
// interrupted, the saved update is all that is needed to resume or revert.
// Content for new digests is not part of the ObjectUpdate: see [ContentMap]
// and [Stage].
type ObjectUpdate struct {
	id       string
	base     *StoredInventory // nil for a new object
	rootSpec Spec             // storage root's spec, if known
	alg      digest.Algorithm // primary digest algorithm for the new version
	state    DigestMap        // new version state: always normalized
	fixity   map[string]digest.Set
	final    *updateFinal // nil for a draft

	// baseInv is a normalized copy of base. It must not be modified.
	baseInv *Inventory
	// newInv is the new inventory, built during finalize.
	newInv *StoredInventory
}

// NewUpdate returns a new draft *ObjectUpdate for the object at dir in fsys.
// If the object exists, the update starts from the object's head version
// state, and the object's ID must match id (if id is not empty). If the
// object doesn't exist, id is required, the update starts with an empty state,
// and the digest algorithm is sha512 unless [UpdateWithDigestAlgorithm] is
// used (it is ignored if the object exists). NewUpdate returns an error if dir
// isn't empty and doesn't hold an OCFL object, or if the object is incomplete
// (an error wrapping [ErrObjectIncomplete]).
func NewUpdate(ctx context.Context, fsys ocflfs.FS, dir, id string, opts ...UpdateOption) (*ObjectUpdate, error) {
	if !fs.ValidPath(dir) {
		return nil, fmt.Errorf("invalid object path: %q: %w", dir, fs.ErrInvalid)
	}
	base, err := readUpdateBase(ctx, fsys, dir)
	if err != nil {
		return nil, err
	}
	o := newUpdateOptions(opts...)
	alg := o.alg
	if base != nil {
		alg = nil // use the object's algorithm
	}
	return newObjectUpdate(id, base, o.rootSpec, alg)
}

// ID returns the ID of the object being updated.
func (u *ObjectUpdate) ID() string { return u.id }

// DigestAlgorithm returns the primary digest algorithm for the new version.
func (u *ObjectUpdate) DigestAlgorithm() digest.Algorithm { return u.alg }

// BaseInventoryDigest returns the digest of the object's inventory.json at the
// time the update was started. It is empty if the update creates a new object.
func (u *ObjectUpdate) BaseInventoryDigest() string {
	if u.base == nil {
		return ""
	}
	return u.base.digest
}

// NextHead returns the version number of the new version.
func (u *ObjectUpdate) NextHead() VNum {
	head, _ := u.nextHead()
	return head
}

// NewState returns a copy of the new version's state. See [ObjectUpdate.BaseState]
// for the state of the version the update is based on.
func (u *ObjectUpdate) NewState() DigestMap { return u.state.Clone() }

// BaseState returns a copy of the object's head version state at the time the
// update was started. It is empty if the update creates a new object. Its
// digests use the update's digest algorithm, so it can be compared with
// [ObjectUpdate.NewState] without reading the object.
func (u *ObjectUpdate) BaseState() DigestMap { return u.baseState().Clone() }

// Fixity returns a copy of the fixity values for new content with the given
// digest. It returns nil for content that is already in the object.
func (u *ObjectUpdate) Fixity(dig string) digest.Set {
	return maps.Clone(u.fixity[normalizeDigest(dig)])
}

// Finalized returns true if u has been finalized.
func (u *ObjectUpdate) Finalized() bool { return u.final != nil }

// NewInventoryDigest returns the digest of the new inventory.json, or an
// empty string if u has not been finalized.
func (u *ObjectUpdate) NewInventoryDigest() string {
	if u.newInv == nil {
		return ""
	}
	return u.newInv.digest
}

// VersionInfo returns the new version's metadata and true if u has been
// finalized, or the zero value and false if u is a draft.
func (u *ObjectUpdate) VersionInfo() (VersionInfo, bool) {
	if u.final == nil {
		return VersionInfo{}, false
	}
	return VersionInfo{
		Message: u.final.Message,
		User:    u.final.User,
		Created: u.final.Created,
		Spec:    u.final.Spec,
	}, true
}

// Add sets the digest for the file name in the new version state, replacing
// any existing digest for name. The digest must use the update's digest
// algorithm. Fixity values for the content are saved unless the content is
// already in the object. fixity may include a value for the update's digest
// algorithm, which isn't saved and must be dig. Add returns an error if name
// is invalid or conflicts with an existing path (a file can't also be a
// directory), and [ErrFinalized] if u has been finalized.
func (u *ObjectUpdate) Add(name, dig string, fixity digest.Set) error {
	return u.addAll([]updateEntry{{name: name, digest: dig, fixity: fixity}})
}

// Remove removes the file or directory name from the new version state. It
// returns an error wrapping [fs.ErrNotExist] if name isn't a file or
// directory in the state and [ErrFinalized] if u has been finalized.
func (u *ObjectUpdate) Remove(name string) error {
	if u.final != nil {
		return ErrFinalized
	}
	if !validPath(name) {
		return &MapPathInvalidErr{Path: name}
	}
	next := DigestMap{}
	var found bool
	for p, dig := range u.state.Paths() {
		if p == name || strings.HasPrefix(p, name+"/") {
			found = true
			continue
		}
		next[dig] = append(next[dig], p)
	}
	if !found {
		return fmt.Errorf("removing %q: %w", name, fs.ErrNotExist)
	}
	u.setState(next)
	return nil
}

// Rename renames the file or directory src in the new version state to dst.
// If src is ".", all files are moved into the directory dst. Rename returns
// an error wrapping [fs.ErrNotExist] if src doesn't exist, an error if the
// result has conflicting paths, and [ErrFinalized] if u has been finalized.
func (u *ObjectUpdate) Rename(src, dst string) error {
	if u.final != nil {
		return ErrFinalized
	}
	if src != "." && !validPath(src) {
		return &MapPathInvalidErr{Path: src}
	}
	if !validPath(dst) {
		return &MapPathInvalidErr{Path: dst}
	}
	var found bool
	for p := range u.state.Paths() {
		if src == "." || p == src || strings.HasPrefix(p, src+"/") {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("renaming %q: %w", src, fs.ErrNotExist)
	}
	next := u.state.Clone()
	next.Mutate(RenamePaths(src, dst))
	norm, err := next.Normalize()
	if err != nil {
		return err
	}
	u.setState(norm)
	return nil
}

// Clear removes all files from the new version state. It returns
// [ErrFinalized] if u has been finalized.
func (u *ObjectUpdate) Clear() error {
	if u.final != nil {
		return ErrFinalized
	}
	u.setState(DigestMap{})
	return nil
}

// Finalize settles the new version's metadata (the message, user, created
// timestamp and OCFL spec) and the content paths for new content, builds the
// new inventory, and makes u read-only. It does no I/O. Finalize uses the
// options [UpdateWithVersionCreated], [UpdateWithOCFLSpec],
// [UpdateWithContentPathFunc], [UpdateWithExpectedHead] and
// [UpdateWithUnchangedVersionState]. If Finalize returns an error, u is
// unchanged.
func (u *ObjectUpdate) Finalize(msg string, user User, opts ...UpdateOption) error {
	if u.final != nil {
		return ErrFinalized
	}
	o := newUpdateOptions(opts...)
	newHead, err := u.nextHead()
	if err != nil {
		return err
	}
	if o.expectedHead > 0 && newHead.Num() != o.expectedHead {
		return fmt.Errorf("%w: expected update to create version %d, but it would create version %d",
			ErrUnexpectedHead, o.expectedHead, newHead.Num())
	}
	if !o.allowUnchanged && u.base != nil && u.baseState().Eq(u.state) {
		return errors.New("update has unchanged version state")
	}
	final := &updateFinal{
		Message:      msg,
		User:         user,
		Spec:         o.spec,
		ContentPaths: DigestMap{},
	}
	if final.Spec.Empty() {
		switch {
		case u.base != nil:
			final.Spec = u.base.Type.Spec
		case !u.rootSpec.Empty():
			final.Spec = u.rootSpec
		default:
			final.Spec = defaultOCFL().Spec()
		}
	}
	created := o.created
	if created.IsZero() {
		created = time.Now()
	}
	// created is saved exactly as it is written to the inventory, so loading a
	// saved update rebuilds the same inventory.
	if final.Created, err = normalizeCreated(created); err != nil {
		return err
	}
	contentDir := u.contentDirectory()
	for dig, paths := range u.state {
		if u.inBase(dig) {
			continue
		}
		paths = slices.Clone(paths)
		if o.contentPathFunc != nil {
			paths = o.contentPathFunc(paths)
		}
		for i, p := range paths {
			paths[i] = path.Join(newHead.String(), contentDir, p)
		}
		slices.Sort(paths)
		final.ContentPaths[dig] = paths
	}
	newInv, err := u.buildInventory(final)
	if err != nil {
		return err
	}
	final.NewInventoryDigest = newInv.digest
	u.final = final
	u.newInv = newInv
	return nil
}

// Apply writes the finalized update to the object at dir in fsys, using src
// for new content, and returns the updated object. If an earlier Apply was
// interrupted, Apply resumes it.
//
// What Apply does depends on the object's root inventory sidecar in storage.
// If it holds the update's base inventory digest, the update has not been
// committed: every step is run again, overwriting anything written by an
// earlier attempt. If it holds the new inventory digest, the update was
// committed and there is nothing to do. Any other value means the object was
// changed by something else, and Apply returns an error wrapping
// [ErrUpdateConflict] without writing anything. For a new object, which has
// no sidecar until the update is committed, dir must also be missing or
// empty, or hold only files and directories that the update writes (an error
// wrapping ErrUpdateConflict otherwise).
//
// Apply returns an error wrapping [ErrMissingContent], without writing
// anything, if src doesn't provide content for every digest that is new in
// the update. src may be nil if the update doesn't add new content. A
// [ContentMap] loaded from JSON must be opened with [ContentMap.OpenFS] first.
// If src is a [ContentFastChecker], such as a ContentMap, Apply returns any error
// from its ContentFastCheck method, without writing anything (this is a fast
// check of file metadata; it does not validate digests): for a ContentMap,
// an error wrapping [ErrContentChanged] if files that new content is in are
// missing or their size or content token has changed. Resuming an update
// copies all new content again, so the check is run then too.
// Apply uses the options [UpdateWithLogger] and [UpdateWithGoLimit].
func (u *ObjectUpdate) Apply(ctx context.Context, fsys ocflfs.FS, dir string, src ContentSource, opts ...UpdateOption) (*Object, error) {
	if u.final == nil {
		return nil, ErrNotFinalized
	}
	writeFS, ok := fsys.(ocflfs.WriteFS)
	if !ok {
		return nil, fmt.Errorf("%q cannot be updated: storage backend does not support writes", u.id)
	}
	o := newUpdateOptions(opts...)
	status, err := u.storageStatus(ctx, fsys, dir)
	if err != nil {
		return nil, err
	}
	if status == updatePending {
		steps := u.applySteps()
		if err := checkContentSource(ctx, steps, src); err != nil {
			return nil, err
		}
		if err := runSteps(ctx, steps, writeFS, dir, src, o.goLimit, o.logger); err != nil {
			return nil, err
		}
	}
	return &Object{fs: fsys, path: dir, inventory: u.newInv}, nil
}

// Revert removes everything written to the object at dir in fsys by an
// interrupted [ObjectUpdate.Apply], restoring the object's base inventory (or,
// for a new object, removing the object directory). On success, u returns to
// being a draft, so it can be edited and finalized again.
//
// Like Apply, Revert decides what to do from the object's root inventory
// sidecar. It returns [ErrRevertUpdate] if the update was committed, and an
// error wrapping [ErrUpdateConflict] if the object was changed by something
// else. For a new object, Revert removes dir only if everything in it could
// have been written by the update, as described for Apply. Revert uses the
// options [UpdateWithLogger] and [UpdateWithGoLimit].
func (u *ObjectUpdate) Revert(ctx context.Context, fsys ocflfs.FS, dir string, opts ...UpdateOption) error {
	if u.final == nil {
		return ErrNotFinalized
	}
	writeFS, ok := fsys.(ocflfs.WriteFS)
	if !ok {
		return fmt.Errorf("%q cannot be reverted: storage backend does not support writes", u.id)
	}
	o := newUpdateOptions(opts...)
	status, err := u.storageStatus(ctx, fsys, dir)
	if err != nil {
		return err
	}
	if status == updateCommitted {
		return ErrRevertUpdate
	}
	if err := runSteps(ctx, u.revertSteps(), writeFS, dir, nil, o.goLimit, o.logger); err != nil {
		return err
	}
	u.final = nil
	u.newInv = nil
	return nil
}

// MarshalJSON implements [json.Marshaler] for ObjectUpdate.
func (u ObjectUpdate) MarshalJSON() ([]byte, error) {
	j := objectUpdateJSON{
		ID:        u.id,
		RootSpec:  u.rootSpec,
		State:     u.state,
		Fixity:    u.fixity,
		Finalized: u.final,
	}
	if u.alg != nil {
		j.DigestAlgorithm = u.alg.ID()
	}
	if j.State == nil {
		j.State = DigestMap{}
	}
	if u.base != nil {
		// the base inventory is saved as a string to preserve its exact bytes:
		// its digest must match the object's sidecar.
		if !utf8.Valid(u.base.bytes) {
			return nil, errors.New("base inventory is not valid UTF-8")
		}
		j.BaseInventory = string(u.base.bytes)
	}
	return json.Marshal(j)
}

// UnmarshalJSON implements [json.Unmarshaler] for *ObjectUpdate. For a
// finalized update, the new inventory is rebuilt from the saved values and
// its digest is compared with the saved digest: if they don't match, the
// saved update was changed (or was saved by a version of this package that
// builds inventories differently) and an error is returned.
func (u *ObjectUpdate) UnmarshalJSON(data []byte) error {
	var j objectUpdateJSON
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&j); err != nil {
		return fmt.Errorf("decoding object update: %w", err)
	}
	var base *StoredInventory
	if j.BaseInventory != "" {
		var err error
		base, err = newStoredInventory([]byte(j.BaseInventory))
		if err != nil {
			return fmt.Errorf("object update's base inventory: %w", err)
		}
	}
	alg, err := digest.DefaultRegistry().Get(j.DigestAlgorithm)
	if err != nil {
		return fmt.Errorf("object update's digest algorithm: %w", err)
	}
	loaded, err := newObjectUpdate(j.ID, base, j.RootSpec, alg)
	if err != nil {
		return err
	}
	if j.State == nil {
		j.State = DigestMap{}
	}
	state, err := j.State.Normalize()
	if err != nil {
		return fmt.Errorf("object update's state: %w", err)
	}
	for dig := range state {
		if err := validDigest(alg, dig); err != nil {
			return fmt.Errorf("object update's state: %w", err)
		}
	}
	loaded.state = state
	for dig, set := range j.Fixity {
		dig = normalizeDigest(dig)
		if !loaded.needsContent(dig) {
			return fmt.Errorf("object update has fixity for content that isn't new: %q", dig)
		}
		if err := loaded.addFixity(dig, set); err != nil {
			return err
		}
	}
	if j.Finalized != nil {
		newInv, err := loaded.buildInventory(j.Finalized)
		if err != nil {
			return fmt.Errorf("rebuilding object update's new inventory: %w", err)
		}
		if newInv.digest != j.Finalized.NewInventoryDigest {
			return fmt.Errorf("object update's new inventory digest doesn't match the saved value: got %q, expected %q",
				newInv.digest, j.Finalized.NewInventoryDigest)
		}
		loaded.final = j.Finalized
		loaded.newInv = newInv
	}
	*u = *loaded
	return nil
}

// addAll adds all entries to u's state, or none of them if there is an
// error.
func (u *ObjectUpdate) addAll(entries []updateEntry) error {
	if u.final != nil {
		return ErrFinalized
	}
	pathMap := u.state.PathMap()
	for i := range entries {
		e := &entries[i]
		if !validPath(e.name) {
			return &MapPathInvalidErr{Path: e.name}
		}
		e.digest = normalizeDigest(e.digest)
		if err := validDigest(u.alg, e.digest); err != nil {
			return fmt.Errorf("adding %q: %w", e.name, err)
		}
		if val, ok := e.fixity[u.alg.ID()]; ok && normalizeDigest(val) != e.digest {
			return fmt.Errorf("adding %q: fixity has a different %s digest: %q", e.name, u.alg.ID(), val)
		}
		pathMap[e.name] = e.digest
	}
	next := pathMap.DigestMap()
	norm, err := next.Normalize()
	if err != nil {
		return err
	}
	// check fixity before changing anything
	fixity := maps.Clone(u.fixity)
	for _, e := range entries {
		if u.inBase(e.digest) || len(e.fixity) == 0 {
			continue
		}
		set := maps.Clone(fixity[e.digest])
		if set == nil {
			set = digest.Set{}
		}
		for alg, val := range e.fixity {
			if alg == u.alg.ID() {
				continue
			}
			if err := set.Add(digest.Set{alg: normalizeDigest(val)}); err != nil {
				return fmt.Errorf("adding %q: conflicting fixity: %w", e.name, err)
			}
		}
		if len(set) > 0 {
			fixity[e.digest] = set
		}
	}
	u.fixity = fixity
	u.setState(norm)
	return nil
}

// setState sets u's state and drops fixity for content that is no longer
// needed. The state must be normalized.
func (u *ObjectUpdate) setState(state DigestMap) {
	u.state = state
	for dig := range u.fixity {
		if !u.needsContent(dig) {
			delete(u.fixity, dig)
		}
	}
}

// addFixity adds fixity values for new content with digest dig.
func (u *ObjectUpdate) addFixity(dig string, set digest.Set) error {
	if len(set) == 0 {
		return nil
	}
	if u.fixity[dig] == nil {
		u.fixity[dig] = digest.Set{}
	}
	for alg, val := range set {
		if alg == u.alg.ID() {
			return fmt.Errorf("fixity for %q includes the primary digest algorithm, %s", dig, alg)
		}
		if err := u.fixity[dig].Add(digest.Set{alg: normalizeDigest(val)}); err != nil {
			return err
		}
	}
	return nil
}

// baseState returns the head version state of the base inventory. It is
// empty for a new object. It must not be modified.
func (u *ObjectUpdate) baseState() DigestMap {
	if u.baseInv == nil {
		return DigestMap{}
	}
	if headVer := u.baseInv.Versions[u.baseInv.Head]; headVer != nil && headVer.State != nil {
		return headVer.State
	}
	return DigestMap{}
}

// inBase returns true if content with the digest is in the base inventory.
func (u *ObjectUpdate) inBase(dig string) bool {
	return u.baseInv != nil && len(u.baseInv.Manifest[dig]) > 0
}

// needsContent returns true if content with the digest is in the new state
// and not in the base inventory.
func (u *ObjectUpdate) needsContent(dig string) bool {
	return len(u.state[dig]) > 0 && !u.inBase(dig)
}

func (u *ObjectUpdate) nextHead() (VNum, error) {
	if u.base == nil {
		return V(1), nil
	}
	head, err := u.base.Head.Next()
	if err != nil {
		return VNum{}, fmt.Errorf("existing inventory's version scheme doesn't support additional versions: %w", err)
	}
	return head, nil
}

func (u *ObjectUpdate) contentDirectory() string {
	if u.base != nil && u.base.ContentDirectory != "" {
		return u.base.ContentDirectory
	}
	return contentDir
}

// buildInventory builds the new inventory from u's base inventory, state,
// fixity and final. It is deterministic: the same inputs result in the same
// inventory bytes.
func (u *ObjectUpdate) buildInventory(final *updateFinal) (*StoredInventory, error) {
	if err := u.checkSpec(final.Spec); err != nil {
		return nil, err
	}
	newHead, err := u.nextHead()
	if err != nil {
		return nil, err
	}
	inv := &Inventory{
		ID:       u.id,
		Manifest: DigestMap{},
		Versions: map[VNum]*InventoryVersion{},
		Fixity:   map[string]DigestMap{},
	}
	if u.baseInv != nil {
		// a fresh copy: u.baseInv must not be modified.
		if inv, err = normalizeInventory(u.baseInv); err != nil {
			return nil, err
		}
	}
	inv.Type = final.Spec.InventoryType()
	inv.DigestAlgorithm = u.alg.ID()
	inv.Head = newHead
	user := final.User
	inv.Versions[newHead] = &InventoryVersion{
		Created: final.Created,
		State:   u.state.Clone(),
		Message: final.Message,
		User:    &user,
	}
	// content paths for new content
	contentPrefix := path.Join(newHead.String(), u.contentDirectory()) + "/"
	for dig := range final.ContentPaths {
		if !u.needsContent(dig) {
			return nil, fmt.Errorf("content paths given for content that isn't new: %q", dig)
		}
	}
	for dig := range u.state {
		if u.inBase(dig) {
			continue
		}
		paths := final.ContentPaths[dig]
		if len(paths) == 0 {
			return nil, fmt.Errorf("missing content paths for new content: %q", dig)
		}
		for _, p := range paths {
			if !strings.HasPrefix(p, contentPrefix) {
				return nil, fmt.Errorf("content path %q is not in %q", p, contentPrefix)
			}
		}
		inv.Manifest[dig] = slices.Sorted(slices.Values(paths))
		for fixAlg, fixDigest := range u.fixity[dig] {
			if inv.Fixity[fixAlg] == nil {
				inv.Fixity[fixAlg] = DigestMap{}
			}
			inv.Fixity[fixAlg][fixDigest] = append(inv.Fixity[fixAlg][fixDigest], paths...)
		}
	}
	for _, fixMap := range inv.Fixity {
		for fixDigest, paths := range fixMap {
			slices.Sort(paths)
			fixMap[fixDigest] = slices.Compact(paths)
		}
	}
	if err := inv.Validate().Err(); err != nil {
		return nil, fmt.Errorf("generated inventory is not valid: %w", err)
	}
	invBytes, invDigest, err := inv.marshal()
	if err != nil {
		return nil, fmt.Errorf("encoding new inventory: %w", err)
	}
	return &StoredInventory{Inventory: *inv, bytes: invBytes, digest: invDigest}, nil
}

// checkSpec returns an error if spec can't be used for the new version.
func (u *ObjectUpdate) checkSpec(spec Spec) error {
	if _, err := getOCFL(spec); err != nil {
		return fmt.Errorf("OCFL v%s: %w", spec, err)
	}
	if !u.rootSpec.Empty() && spec.Cmp(u.rootSpec) > 0 {
		return fmt.Errorf("%w: object spec is OCFL v%s, storage root spec is OCFL v%s",
			ErrObjectSpecExceedsRoot, spec, u.rootSpec)
	}
	if u.base != nil && spec.Cmp(u.base.Type.Spec) < 0 {
		return fmt.Errorf("new version's OCFL spec (%q) cannot be lower than the previous version's (%q)",
			spec, u.base.Type.Spec)
	}
	return nil
}

// storageStatus reads the root inventory sidecar for the object at dir and
// reports whether the update is pending or committed. It returns an error
// wrapping ErrUpdateConflict if the object matches neither the base inventory
// nor the new inventory.
func (u *ObjectUpdate) storageStatus(ctx context.Context, fsys ocflfs.FS, dir string) (updateStatus, error) {
	sidecar, err := readSidecarIfExists(ctx, fsys, dir, u.newInv.DigestAlgorithm)
	if err != nil {
		return 0, err
	}
	if strings.EqualFold(sidecar, u.newInv.digest) {
		return updateCommitted, nil
	}
	if u.base == nil {
		if sidecar != "" {
			return 0, fmt.Errorf("%w: object has an inventory sidecar but the update is for a new object", ErrUpdateConflict)
		}
		if err := u.checkNewObjectDir(ctx, fsys, dir); err != nil {
			return 0, err
		}
		return updatePending, nil
	}
	if strings.EqualFold(sidecar, u.base.digest) {
		return updatePending, nil
	}
	return 0, fmt.Errorf("%w: root inventory sidecar doesn't match the update's base inventory or new inventory", ErrUpdateConflict)
}

// checkNewObjectDir returns an error wrapping ErrUpdateConflict unless
// everything in the object directory dir could have been written by u, an
// update for a new object: dir may be missing or empty, or hold only u's
// object declaration, inventory.json with u's new inventory, the inventory
// sidecar, and u's version directory. Temporary files left by an interrupted
// write of one of those files (".<name>.tmp-*", as local.FS names them) are
// allowed too. Revert removes dir, so it must not hold anything else.
func (u *ObjectUpdate) checkNewObjectDir(ctx context.Context, fsys ocflfs.FS, dir string) error {
	entries, err := ocflfs.ReadDir(ctx, fsys, dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading object root directory: %w", err)
	}
	decl := Namaste{Type: NamasteTypeObject, Version: u.newInv.Type.Spec}
	rootFiles := []string{decl.Name(), inventoryBase, inventoryBase + "." + u.newInv.DigestAlgorithm}
	isTemp := func(name string) bool {
		return slices.ContainsFunc(rootFiles, func(f string) bool {
			return strings.HasPrefix(name, "."+f+".tmp-")
		})
	}
	var hasInventory bool
	for _, e := range entries {
		name := e.Name()
		switch {
		case e.IsDir() && name == u.newInv.Head.String():
		case !e.IsDir() && (slices.Contains(rootFiles, name) || isTemp(name)):
			hasInventory = hasInventory || name == inventoryBase
		default:
			return fmt.Errorf("%w: object directory has %q, which the update doesn't write", ErrUpdateConflict, name)
		}
	}
	if !hasInventory {
		return nil
	}
	invBytes, err := ocflfs.ReadAll(ctx, fsys, path.Join(dir, inventoryBase))
	if err != nil {
		return err
	}
	if !bytes.Equal(invBytes, u.newInv.bytes) {
		return fmt.Errorf("%w: object has an inventory.json that wasn't written by the update", ErrUpdateConflict)
	}
	return nil
}

// applySteps returns the steps for applying u to storage. The step that
// writes the new root inventory sidecar commits the update.
func (u *ObjectUpdate) applySteps() []updateStep {
	newInv := u.newInv
	newHead := newInv.Head
	newAlg := newInv.DigestAlgorithm
	var oldSpec Spec
	if u.base != nil {
		oldSpec = u.base.Type.Spec
	}
	var steps []updateStep
	// object declaration
	if newSpec := newInv.Type.Spec; newSpec != oldSpec {
		newDecl := Namaste{Type: NamasteTypeObject, Version: newSpec}
		steps = append(steps, updateStep{
			name: "write " + newDecl.Name(),
			run: func(ctx context.Context, fsys ocflfs.WriteFS, dir string, _ ContentSource) error {
				return WriteDeclaration(ctx, fsys, dir, newDecl)
			},
		})
		if !oldSpec.Empty() {
			oldDecl := Namaste{Type: NamasteTypeObject, Version: oldSpec}
			steps = append(steps, updateStep{
				name: "remove " + oldDecl.Name(),
				run: func(ctx context.Context, fsys ocflfs.WriteFS, dir string, _ ContentSource) error {
					return removeIfExists(ctx, fsys, path.Join(dir, oldDecl.Name()))
				},
			})
		}
	}
	// version content
	for contentPath, dig := range newInv.versionContent(newHead).SortedPaths() {
		steps = append(steps, updateStep{
			name:   "copy " + contentPath,
			digest: dig,
			async:  true,
			run: func(ctx context.Context, fsys ocflfs.WriteFS, dir string, src ContentSource) error {
				srcFS, srcPath := getContent(src, dig)
				if srcFS == nil {
					return fmt.Errorf("%w: %q", ErrMissingContent, dig)
				}
				_, err := ocflfs.Copy(ctx, fsys, path.Join(dir, contentPath), srcFS, srcPath)
				return err
			},
		})
	}
	// version inventory and sidecar
	verDir := newHead.String()
	steps = append(steps,
		updateStep{
			name: "write " + path.Join(verDir, inventoryBase),
			run: func(ctx context.Context, fsys ocflfs.WriteFS, dir string, _ ContentSource) error {
				_, err := fsys.Write(ctx, path.Join(dir, verDir, inventoryBase), bytes.NewReader(newInv.bytes))
				return err
			},
		},
		updateStep{
			name: "write " + path.Join(verDir, inventoryBase+"."+newAlg),
			run: func(ctx context.Context, fsys ocflfs.WriteFS, dir string, _ ContentSource) error {
				return writeInventorySidecar(ctx, fsys, path.Join(dir, verDir), newInv.digest, newAlg)
			},
		},
		// root inventory and sidecar
		updateStep{
			name: "write " + inventoryBase,
			run: func(ctx context.Context, fsys ocflfs.WriteFS, dir string, _ ContentSource) error {
				_, err := fsys.Write(ctx, path.Join(dir, inventoryBase), bytes.NewReader(newInv.bytes))
				return err
			},
		},
		updateStep{
			name: "write " + inventoryBase + "." + newAlg,
			run: func(ctx context.Context, fsys ocflfs.WriteFS, dir string, _ ContentSource) error {
				return writeInventorySidecar(ctx, fsys, dir, newInv.digest, newAlg)
			},
		},
	)
	return steps
}

// revertSteps returns steps for undoing an uncommitted update. They must not
// change the root inventory sidecar, so an interrupted revert can be run
// again.
func (u *ObjectUpdate) revertSteps() []updateStep {
	if u.base == nil {
		return []updateStep{{
			name: "remove object directory",
			run: func(ctx context.Context, fsys ocflfs.WriteFS, dir string, _ ContentSource) error {
				return fsys.RemoveAll(ctx, dir)
			},
		}}
	}
	base := u.base
	verDir := u.newInv.Head.String()
	steps := []updateStep{
		{
			name: "restore " + inventoryBase,
			run: func(ctx context.Context, fsys ocflfs.WriteFS, dir string, _ ContentSource) error {
				_, err := fsys.Write(ctx, path.Join(dir, inventoryBase), bytes.NewReader(base.bytes))
				return err
			},
		},
		{
			name: "remove version directory " + verDir,
			run: func(ctx context.Context, fsys ocflfs.WriteFS, dir string, _ ContentSource) error {
				return fsys.RemoveAll(ctx, path.Join(dir, verDir))
			},
		},
	}
	if newSpec, oldSpec := u.newInv.Type.Spec, base.Type.Spec; newSpec != oldSpec {
		oldDecl := Namaste{Type: NamasteTypeObject, Version: oldSpec}
		newDecl := Namaste{Type: NamasteTypeObject, Version: newSpec}
		steps = append(steps,
			updateStep{
				name: "write " + oldDecl.Name(),
				run: func(ctx context.Context, fsys ocflfs.WriteFS, dir string, _ ContentSource) error {
					return WriteDeclaration(ctx, fsys, dir, oldDecl)
				},
			},
			updateStep{
				name: "remove " + newDecl.Name(),
				run: func(ctx context.Context, fsys ocflfs.WriteFS, dir string, _ ContentSource) error {
					return removeIfExists(ctx, fsys, path.Join(dir, newDecl.Name()))
				},
			},
		)
	}
	return steps
}

// UpdateOption is an optional argument for creating, finalizing, applying
// and reverting an [ObjectUpdate]. Each function documents the options it
// uses; others are ignored.
type UpdateOption func(*updateOptions)

type updateOptions struct {
	// constructors
	alg      digest.Algorithm
	rootSpec Spec
	// finalize
	created         time.Time
	spec            Spec
	expectedHead    int
	allowUnchanged  bool
	contentPathFunc PathMutation
	// apply and revert
	logger  *slog.Logger
	goLimit int
}

func newUpdateOptions(opts ...UpdateOption) *updateOptions {
	o := &updateOptions{logger: logging.DisabledLogger()}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// UpdateWithDigestAlgorithm sets the primary digest algorithm (sha512 or
// sha256) for a new object. Without it, a new object uses sha512. It is
// ignored if the object exists: the update uses the object's algorithm.
// Changing an existing object's digest algorithm isn't supported.
func UpdateWithDigestAlgorithm(alg digest.Algorithm) UpdateOption {
	return func(o *updateOptions) {
		o.alg = alg
	}
}

// UpdateWithVersionCreated sets the 'created' timestamp for the new version.
// It is used by [ObjectUpdate.Finalize]. The timestamp is truncated to the
// second. Without it, the current time is used.
func UpdateWithVersionCreated(t time.Time) UpdateOption {
	return func(o *updateOptions) {
		o.created = t
	}
}

// UpdateWithOCFLSpec sets the OCFL specification for the new object version.
// It is used by [ObjectUpdate.Finalize]. Without it, an existing object keeps
// its spec, and a new object uses its storage root's spec (if known) or the
// latest spec.
func UpdateWithOCFLSpec(s Spec) UpdateOption {
	return func(o *updateOptions) {
		o.spec = s
	}
}

// UpdateWithExpectedHead is used to enforce the expected version number (without
// padding) for the new version. It is used by [ObjectUpdate.Finalize], which
// returns an error wrapping [ErrUnexpectedHead] if the update would create a
// version with a different number. For a new object, the expected version
// number is 1. Values of v less than 1 are ignored.
func UpdateWithExpectedHead(v int) UpdateOption {
	return func(o *updateOptions) {
		o.expectedHead = v
	}
}

// UpdateWithUnchangedVersionState allows updates that don't change the
// version state. Without it, [ObjectUpdate.Finalize] returns an error if the
// new version state is the same as the head version's.
func UpdateWithUnchangedVersionState() UpdateOption {
	return func(o *updateOptions) {
		o.allowUnchanged = true
	}
}

// UpdateWithContentPathFunc sets a function for enforcing naming conventions
// for content paths in the new inventory. The function is called by
// [ObjectUpdate.Finalize] with the logical paths for each new digest.
func UpdateWithContentPathFunc(mutate PathMutation) UpdateOption {
	return func(o *updateOptions) {
		o.contentPathFunc = mutate
	}
}

// UpdateWithLogger sets the logger used to log each step of
// [ObjectUpdate.Apply] and [ObjectUpdate.Revert].
func UpdateWithLogger(logger *slog.Logger) UpdateOption {
	return func(o *updateOptions) {
		if logger != nil {
			o.logger = logger
		}
	}
}

// UpdateWithGoLimit sets the number of goroutines used by
// [ObjectUpdate.Apply] to copy content concurrently. The default is
// runtime.NumCPU().
func UpdateWithGoLimit(gos int) UpdateOption {
	return func(o *updateOptions) {
		o.goLimit = gos
	}
}

// updateWithRootSpec sets the storage root's spec for a new ObjectUpdate.
func updateWithRootSpec(spec Spec) UpdateOption {
	return func(o *updateOptions) {
		o.rootSpec = spec
	}
}

// newObjectUpdate returns a new draft for an object with the given base
// inventory (nil for a new object). If alg is nil, the base inventory's
// algorithm or sha512 is used. If base and alg are both set, alg must be the
// base inventory's algorithm.
func newObjectUpdate(id string, base *StoredInventory, rootSpec Spec, alg digest.Algorithm) (*ObjectUpdate, error) {
	if base != nil {
		if id != "" && id != base.ID {
			return nil, fmt.Errorf("object has unexpected ID: %q; expected: %q", base.ID, id)
		}
		id = base.ID
		if alg != nil && alg.ID() != base.DigestAlgorithm {
			return nil, fmt.Errorf("digest algorithm %s doesn't match the object's digest algorithm, %s: changing an existing object's digest algorithm isn't supported",
				alg.ID(), base.DigestAlgorithm)
		}
		var err error
		if alg, err = digest.DefaultRegistry().Get(base.DigestAlgorithm); err != nil {
			return nil, err
		}
	}
	if id == "" {
		return nil, ErrNoObjectID
	}
	if alg == nil {
		alg = digest.SHA512
	}
	if err := validUpdateAlgorithm(alg); err != nil {
		return nil, err
	}
	u := &ObjectUpdate{
		id:       id,
		base:     base,
		rootSpec: rootSpec,
		alg:      alg,
		state:    DigestMap{},
		fixity:   map[string]digest.Set{},
	}
	if base != nil {
		var err error
		u.baseInv, err = normalizeInventory(&base.Inventory)
		if err != nil {
			return nil, err
		}
		u.state = u.baseState().Clone()
	}
	return u, nil
}

// updateEntry is a file added to an update's state
type updateEntry struct {
	name   string
	digest string
	fixity digest.Set
}

// readUpdateBase reads and validates the root inventory for the object at
// dir. It returns nil if dir doesn't exist or is empty.
func readUpdateBase(ctx context.Context, fsys ocflfs.FS, dir string) (*StoredInventory, error) {
	entries, err := ocflfs.ReadDir(ctx, fsys, dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("reading object root directory: %w", err)
	}
	if len(entries) == 0 {
		return nil, nil
	}
	dirState := ParseObjectDir(entries)
	if !dirState.HasNamaste() {
		return nil, errors.New("directory is not empty: non-conforming contents")
	}
	inv, err := ReadInventory(ctx, fsys, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: missing %s", ErrObjectIncomplete, inventoryBase)
		}
		return nil, err
	}
	if err := inv.ValidateSidecar(ctx, fsys, dir); err != nil {
		var digestErr *digest.DigestError
		if errors.Is(err, fs.ErrNotExist) || errors.As(err, &digestErr) {
			return nil, fmt.Errorf("%w: %w", ErrObjectIncomplete, err)
		}
		return nil, err
	}
	// an update interrupted before writing the root inventory may have left
	// a new version directory, declaration, or sidecar.
	if next, err := inv.Head.Next(); err == nil && dirState.HasVersionDir(next) {
		return nil, fmt.Errorf("%w: version directory %s isn't in the inventory", ErrObjectIncomplete, next)
	}
	if dirState.Spec != inv.Type.Spec {
		return nil, fmt.Errorf("%w: object declaration (OCFL v%s) doesn't match the inventory (OCFL v%s)",
			ErrObjectIncomplete, dirState.Spec, inv.Type.Spec)
	}
	for _, name := range dirState.Invalid {
		if strings.HasPrefix(name, objectDeclPrefix) || strings.HasPrefix(name, sidecarPrefix) {
			return nil, fmt.Errorf("%w: unexpected %s", ErrObjectIncomplete, name)
		}
	}
	return inv, nil
}

// updateStatus describes the state of an update in storage
type updateStatus int

const (
	updatePending   updateStatus = iota // not committed: may be partially applied
	updateCommitted                     // new root inventory sidecar was written
)

// updateStep is a single step in applying or reverting an update. Steps must
// be safe to run more than once.
type updateStep struct {
	name   string
	digest string // digest of content copied in the step, if any
	async  bool   // run concurrently with adjacent async steps
	run    func(ctx context.Context, fsys ocflfs.WriteFS, dir string, src ContentSource) error
}

// objectUpdateJSON is the saved form of an ObjectUpdate
type objectUpdateJSON struct {
	ID              string                `json:"id"`
	BaseInventory   string                `json:"base_inventory,omitempty"`
	RootSpec        Spec                  `json:"root_spec,omitempty"`
	DigestAlgorithm string                `json:"digest_algorithm"`
	State           DigestMap             `json:"state"`
	Fixity          map[string]digest.Set `json:"fixity,omitempty"`
	Finalized       *updateFinal          `json:"finalized,omitempty"`
}

// updateFinal holds the values settled by ObjectUpdate.Finalize: together
// with the base inventory, state and fixity, they determine the new
// inventory.
type updateFinal struct {
	Message string    `json:"message"`
	User    User      `json:"user"`
	Created time.Time `json:"created"`
	Spec    Spec      `json:"spec"`
	// ContentPaths are the manifest paths for content that is new in the
	// version.
	ContentPaths       DigestMap `json:"content_paths"`
	NewInventoryDigest string    `json:"new_inventory_digest"`
}

// readSidecarIfExists reads the root inventory sidecar using alg in dir. It
// returns an empty string if the sidecar doesn't exist.
func readSidecarIfExists(ctx context.Context, fsys ocflfs.FS, dir, alg string) (string, error) {
	sum, err := ReadInventorySidecar(ctx, fsys, dir, alg)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return sum, err
}

// checkContentSource returns an error wrapping ErrMissingContent if src
// doesn't provide content for every step that copies content. If src is a
// ContentFastChecker, it returns the error from checking the content.
func checkContentSource(ctx context.Context, steps []updateStep, src ContentSource) error {
	var digests, missing []string
	seen := map[string]bool{}
	for _, step := range steps {
		if step.digest == "" || seen[step.digest] {
			continue
		}
		seen[step.digest] = true
		digests = append(digests, step.digest)
		if srcFS, _ := getContent(src, step.digest); srcFS == nil {
			missing = append(missing, step.digest)
		}
	}
	if len(missing) == 0 {
		if checker, ok := src.(ContentFastChecker); ok {
			return checker.ContentFastCheck(ctx, digests)
		}
		return nil
	}
	listed := missing
	var more string
	if len(missing) > maxMissingContentListed {
		listed = missing[:maxMissingContentListed]
		more = fmt.Sprintf(" and %d more", len(missing)-maxMissingContentListed)
	}
	return fmt.Errorf("%w: %d missing digest(s): %s%s", ErrMissingContent,
		len(missing), strings.Join(listed, ", "), more)
}

// getContent returns the FS and path for content with the digest from src,
// which may be nil. The returned FS is nil if the content isn't available.
func getContent(src ContentSource, dig string) (ocflfs.FS, string) {
	if src == nil {
		return nil, ""
	}
	fsys, name := src.GetContent(dig)
	if fsys == nil || name == "" {
		return nil, ""
	}
	return fsys, name
}

// runSteps runs steps in order. Consecutive async steps run concurrently,
// using up to gos goroutines. It stops at the first error.
//
// A step isn't started, or logged, once ctx is done. A step error wrapping
// [context.Canceled] or [context.DeadlineExceeded] is logged at Debug level,
// not Error, leaving the caller to report the interruption.
func runSteps(ctx context.Context, steps []updateStep, fsys ocflfs.WriteFS, dir string, src ContentSource, gos int, logger *slog.Logger) error {
	if gos < 1 {
		gos = runtime.NumCPU()
	}
	runStep := func(ctx context.Context, step updateStep) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		logger.Info(step.name)
		if err := step.run(ctx, fsys, dir, src); err != nil {
			err = fmt.Errorf("%s: %w", step.name, err)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logger.Debug(err.Error())
			} else {
				logger.Error(err.Error())
			}
			return err
		}
		return nil
	}
	var group *errgroup.Group
	var groupCtx context.Context
	for _, step := range steps {
		if step.async {
			if group == nil {
				// If any of the consecutive async steps returns an error, the
				// context for all of them is canceled.
				group, groupCtx = errgroup.WithContext(ctx)
				group.SetLimit(gos)
			}
			group.Go(func() error { return runStep(groupCtx, step) })
			continue
		}
		// wait for any previous async steps to complete
		if group != nil {
			if err := group.Wait(); err != nil {
				return err
			}
			group = nil
		}
		if err := runStep(ctx, step); err != nil {
			return err
		}
	}
	if group != nil {
		return group.Wait()
	}
	return nil
}

func removeIfExists(ctx context.Context, fsys ocflfs.WriteFS, name string) error {
	err := fsys.Remove(ctx, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// normalizeInventory returns a normalized copy of inv.
func normalizeInventory(inv *Inventory) (*Inventory, error) {
	out := &Inventory{
		ID:               inv.ID,
		Type:             inv.Type,
		DigestAlgorithm:  inv.DigestAlgorithm,
		Head:             inv.Head,
		ContentDirectory: inv.ContentDirectory,
		Versions:         make(map[VNum]*InventoryVersion, len(inv.Versions)+1),
		Fixity:           make(map[string]DigestMap, len(inv.Fixity)+1),
	}
	var err error
	if out.Manifest, err = inv.Manifest.Normalize(); err != nil {
		return nil, fmt.Errorf("in existing inventory manifest: %w", err)
	}
	for fixAlg, fixMap := range inv.Fixity {
		if out.Fixity[fixAlg], err = fixMap.Normalize(); err != nil {
			return nil, fmt.Errorf("in existing inventory %s fixity: %w", fixAlg, err)
		}
	}
	for vnum, ver := range inv.Versions {
		newVer := &InventoryVersion{Created: ver.Created, Message: ver.Message}
		if newVer.State, err = ver.State.Normalize(); err != nil {
			return nil, fmt.Errorf("in existing inventory %s state: %w", vnum, err)
		}
		if ver.User != nil {
			user := *ver.User
			newVer.User = &user
		}
		out.Versions[vnum] = newVer
	}
	return out, nil
}

// normalizeCreated returns t as it will be read back from an inventory: with
// a precision of one second and the time zone as it is encoded in JSON.
func normalizeCreated(t time.Time) (time.Time, error) {
	b, err := t.Truncate(time.Second).MarshalJSON()
	if err != nil {
		return time.Time{}, fmt.Errorf("version created timestamp: %w", err)
	}
	var out time.Time
	if err := out.UnmarshalJSON(b); err != nil {
		return time.Time{}, fmt.Errorf("version created timestamp: %w", err)
	}
	return out, nil
}

// validUpdateAlgorithm returns an error unless alg is sha512 or sha256.
func validUpdateAlgorithm(alg digest.Algorithm) error {
	if alg == nil {
		return errors.New("digest algorithm is not set: must be 'sha512' or 'sha256'")
	}
	if id := alg.ID(); id != digest.SHA512.ID() && id != digest.SHA256.ID() {
		return fmt.Errorf("digest algorithm must be 'sha512' or 'sha256', not %q", id)
	}
	return nil
}

// validDigest returns an error if dig isn't a lowercase hex digest with the
// length of digests from alg (sha512 or sha256).
func validDigest(alg digest.Algorithm, dig string) error {
	size := 128
	if alg.ID() == digest.SHA256.ID() {
		size = 64
	}
	if len(dig) != size || strings.Trim(dig, "0123456789abcdef") != "" {
		return fmt.Errorf("invalid %s digest: %q", alg.ID(), dig)
	}
	return nil
}
