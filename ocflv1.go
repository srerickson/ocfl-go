package ocfl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/extension"
	"github.com/srerickson/ocfl-go/logging"
	"github.com/srerickson/ocfl-go/validation"
	"github.com/srerickson/ocfl-go/validation/code"
	"golang.org/x/sync/errgroup"
)

// ocflv1 is animplementation of ocfl v1.x
type ocflV1 struct {
	spec Spec
}

func (imp ocflV1) Spec() Spec {
	return Spec(imp.spec)
}

func (imp ocflV1) NewInventory(byts []byte) (Inventory, error) {
	inv := &inventoryV1{}
	dec := json.NewDecoder(bytes.NewReader(byts))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&inv.raw); err != nil {
		return nil, err
	}
	if err := inv.setJsonDigest(byts); err != nil {
		return nil, err
	}
	if err := inv.Validate().Err(); err != nil {
		return nil, err
	}
	return inv, nil
}

func (imp ocflV1) ValidateInventoryBytes(raw []byte) (Inventory, *Validation) {
	specStr := string(imp.spec)
	v := &Validation{}
	invMap := map[string]any{}
	if err := json.Unmarshal(raw, &invMap); err != nil {
		err = fmt.Errorf("decoding inventory json: %w", err)
		v.AddFatal(verr(err, code.E033(specStr)))
		return nil, v
	}
	const requiredErrMsg = "required field is missing or has unexpected json value"
	const optionalErrMsg = "optional field has unexpected json value"
	id, exists, typeOK := jsonMapGet[string](invMap, `id`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'id'`)
		v.AddFatal(verr(err, code.E036(specStr)))
	}
	typeStr, exists, typeOK := jsonMapGet[string](invMap, `type`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'type'`)
		v.AddFatal(verr(err, code.E036(specStr)))
	}
	if typeStr != "" && typeStr != Spec(imp.spec).AsInvType().String() {
		err := fmt.Errorf("invalid inventory type value: %q", typeStr)
		v.AddFatal(verr(err, code.E038(specStr)))
	}
	digestAlg, exists, typeOK := jsonMapGet[string](invMap, `digestAlgorithm`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'digestAlgorithm'`)
		v.AddFatal(verr(err, code.E036(specStr)))
	}
	if digestAlg != "" && digestAlg != digest.SHA512.ID() && digestAlg != digest.SHA256.ID() {
		err := fmt.Errorf("invalid digest algorithm: %q", digestAlg)
		v.AddFatal(verr(err, code.E025(specStr)))
	}
	head, exists, typeOK := jsonMapGet[string](invMap, `head`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'head'`)
		v.AddFatal(verr(err, code.E036(specStr)))
	}
	manifestVals, exists, typeOK := jsonMapGet[map[string]any](invMap, `manifest`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'manifest'`)
		v.AddFatal(verr(err, code.E041(specStr)))
	}
	versionsVals, exists, typeOK := jsonMapGet[map[string]any](invMap, `versions`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'versions'`)
		v.AddFatal(verr(err, code.E043(specStr)))
	}
	// FIXME: not sure which error code. E108?
	contentDirectory, exists, typeOK := jsonMapGet[string](invMap, `contentDirectory`)
	if exists && !typeOK {
		// contentDirectory is optional
		err := errors.New(optionalErrMsg + `: 'contentDirectory'`)
		v.AddFatal(err)
	}
	// fixity is optional
	fixityVals, exists, typeOK := jsonMapGet[map[string]any](invMap, `fixity`)
	if exists && !typeOK {
		err := errors.New(optionalErrMsg + `: 'fixity'`)
		v.AddFatal(verr(err, code.E111(specStr)))
	}
	// any remaining values in invVals are invalid
	for extra := range invMap {
		err := fmt.Errorf("inventory json has unexpected field: %q", extra)
		v.AddFatal(err)
	}
	inv := &inventoryV1{
		raw: rawInventory{
			ID:               id,
			ContentDirectory: contentDirectory,
			DigestAlgorithm:  digestAlg,
			Fixity:           map[string]DigestMap{},
			Versions:         make(map[VNum]*rawInventoryVersion),
		},
	}
	if err := inv.raw.Type.UnmarshalText([]byte(typeStr)); err != nil {
		v.AddFatal(verr(err, code.E038(specStr)))
	}
	if err := inv.raw.Head.UnmarshalText([]byte(head)); err != nil {
		v.AddFatal(verr(err, code.E040(specStr)))
	}
	var err error
	if inv.raw.Manifest, err = convertJSONDigestMap(manifestVals); err != nil {
		err = fmt.Errorf("invalid manifest: %w", err)
		v.AddFatal(verr(err, code.E092(specStr)))
	}
	// build versions
	for vnumStr, val := range versionsVals {
		var (
			vnum        VNum
			versionVals map[string]any
			userVals    map[string]any
			stateVals   map[string]any
			createdStr  string
			created     time.Time
			message     string
			state       DigestMap
			user        *User
		)
		if err := ParseVNum(vnumStr, &vnum); err != nil {
			err = fmt.Errorf("invalid key %q in versions block: %w", vnumStr, err)
			v.AddFatal(verr(err, code.E046(specStr)))
			continue
		}
		versionErrPrefix := "version '" + vnumStr + "'"
		versionVals, typeOK = val.(map[string]any)
		if !typeOK {
			err := errors.New(versionErrPrefix + ": value is not a json object")
			v.AddFatal(verr(err, code.E045(specStr)))
		}
		createdStr, exists, typeOK = jsonMapGet[string](versionVals, `created`)
		if !exists || !typeOK {
			err := fmt.Errorf("%s: %s: %s", versionErrPrefix, requiredErrMsg, `'created'`)
			v.AddFatal(verr(err, code.E048(specStr)))
		}
		if createdStr != "" {
			if err := created.UnmarshalText([]byte(createdStr)); err != nil {
				err = fmt.Errorf("%s: created: %w", versionErrPrefix, err)
				v.AddFatal(verr(err, code.E049(specStr)))
			}
		}
		stateVals, exists, typeOK = jsonMapGet[map[string]any](versionVals, `state`)
		if !exists || !typeOK {
			err := fmt.Errorf("%s: %s: %q", versionErrPrefix, requiredErrMsg, `state`)
			v.AddFatal(verr(err, code.E048(specStr)))
		}
		// message is optional
		message, exists, typeOK = jsonMapGet[string](versionVals, `message`)
		if exists && !typeOK {
			err := fmt.Errorf("%s: %s: %q", versionErrPrefix, optionalErrMsg, `message`)
			v.AddFatal(verr(err, code.E094(specStr)))
		}
		// user is optional
		userVals, exists, typeOK := jsonMapGet[map[string]any](versionVals, `user`)
		switch {
		case exists && !typeOK:
			err := fmt.Errorf("%s: %s: %q", versionErrPrefix, optionalErrMsg, `user`)
			v.AddFatal(verr(err, code.E054(specStr)))
		case exists:
			var userName, userAddress string
			userName, exists, typeOK = jsonMapGet[string](userVals, `name`)
			if !exists || !typeOK {
				err := fmt.Errorf("%s: user: %s: %q", versionErrPrefix, requiredErrMsg, `name`)
				v.AddFatal(verr(err, code.E054(specStr)))
			}
			// address is optional
			userAddress, exists, typeOK = jsonMapGet[string](userVals, `address`)
			if exists && !typeOK {
				err := fmt.Errorf("%s: user: %s: %q", versionErrPrefix, optionalErrMsg, `address`)
				v.AddFatal(verr(err, code.E054(specStr)))
			}
			user = &User{Name: userName, Address: userAddress}
		}
		// any additional fields in versionVals are invalid.
		for extra := range versionVals {
			err := fmt.Errorf("%s: invalid key: %q", versionErrPrefix, extra)
			v.AddFatal(err)
		}
		state, err := convertJSONDigestMap(stateVals)
		if err != nil {
			err = fmt.Errorf("%s: state: %w", versionErrPrefix, err)
			v.AddFatal(err)
		}
		inv.raw.Versions[vnum] = &rawInventoryVersion{
			Created: created,
			State:   state,
			Message: message,
			User:    user,
		}
	}
	// build fixity
	for algStr, val := range fixityVals {
		var digestVals map[string]any
		digestVals, typeOK = val.(map[string]any)
		fixityErrPrefix := "fixity '" + algStr + "'"
		if !typeOK {
			err := fmt.Errorf("%s: value is not a json object", fixityErrPrefix)
			v.AddFatal(verr(err, code.E057(specStr)))
			continue
		}
		digests, err := convertJSONDigestMap(digestVals)
		if err != nil {
			err = fmt.Errorf("%s: %w", fixityErrPrefix, err)
			v.AddFatal(verr(err, code.E057(specStr)))
			continue
		}
		inv.raw.Fixity[algStr] = digests
	}
	if err := inv.setJsonDigest(raw); err != nil {
		v.AddFatal(err)
	}
	v.Add(validateInventoryV1(inv, imp.spec))
	if v.Err() != nil {
		return nil, v
	}
	return inv, v
}

func (imp *ocflV1) Commit(ctx context.Context, obj *Object, commit *Commit) error {
	writeFS, ok := obj.FS().(WriteFS)
	if !ok {
		err := errors.New("object's backing file system doesn't support write operations")
		return &CommitError{Err: err}
	}
	newInv, err := buildInventory(obj.Inventory(), commit)
	if err != nil {
		err := fmt.Errorf("building new inventory: %w", err)
		return &CommitError{Err: err}
	}
	logger := commit.Logger
	if logger == nil {
		logger = logging.DisabledLogger()
	}
	logger = logger.With("path", obj.Path(), "id", newInv.ID, "head", newInv.Head, "ocfl_spec", newInv.Spec(), "alg", newInv.DigestAlgorithm)
	// xfers is a subeset of the manifest with the new content to add
	xfers, err := newContentMap(&newInv.raw)
	if err != nil {
		return &CommitError{Err: err}
	}
	// check that the stage includes all the new content
	for digest := range xfers {
		if !commit.Stage.HasContent(digest) {
			// FIXME short digest
			err := fmt.Errorf("no content for digest: %s", digest)
			return &CommitError{Err: err}
		}
	}
	// file changes start here
	// 1. create or update NAMASTE object declaration
	var oldSpec Spec
	if obj.Inventory() != nil {
		oldSpec = obj.Inventory().Spec()
	}
	newSpec := newInv.Spec()
	switch {
	case obj.Exists() && oldSpec != newSpec:
		oldDecl := Namaste{Type: NamasteTypeObject, Version: oldSpec}
		logger.DebugContext(ctx, "deleting previous OCFL object declaration", "name", oldDecl)
		if err = writeFS.Remove(ctx, path.Join(obj.Path(), oldDecl.Name())); err != nil {
			return &CommitError{Err: err, Dirty: true}
		}
		fallthrough
	case !obj.Exists():
		newDecl := Namaste{Type: NamasteTypeObject, Version: newSpec}
		logger.DebugContext(ctx, "writing new OCFL object declaration", "name", newDecl)
		if err = WriteDeclaration(ctx, writeFS, obj.Path(), newDecl); err != nil {
			return &CommitError{Err: err, Dirty: true}
		}
	}
	// 2. tranfser files from stage to object
	if len(xfers) > 0 {
		copyOpts := &copyContentOpts{
			Source:   commit.Stage,
			DestFS:   writeFS,
			DestRoot: obj.Path(),
			Manifest: xfers,
		}
		logger.DebugContext(ctx, "copying new object files", "count", len(xfers))
		if err := copyContent(ctx, copyOpts); err != nil {
			err = fmt.Errorf("transferring new object contents: %w", err)
			return &CommitError{Err: err, Dirty: true}
		}
	}
	logger.DebugContext(ctx, "writing inventories for new object version")
	// 3. write inventory to both object root and version directory
	newVersionDir := path.Join(obj.Path(), newInv.Head().String())
	if err := writeInventory(ctx, writeFS, newInv, obj.Path(), newVersionDir); err != nil {
		err = fmt.Errorf("writing new inventories or inventory sidecars: %w", err)
		return &CommitError{Err: err, Dirty: true}
	}
	obj.inventory = newInv
	obj.ocfl = imp
	return nil
}

func (imp *ocflV1) ValidateObjectRoot(ctx context.Context, fsys FS, dir string, state *ObjectState, vldr *ObjectValidation) (*Object, error) {
	// validate namaste
	specStr := string(imp.spec)
	decl := Namaste{Type: NamasteTypeObject, Version: imp.spec}
	name := path.Join(dir, decl.Name())
	err := ValidateNamaste(ctx, fsys, name)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			err = fmt.Errorf("%s: %w", name, ErrObjectNamasteNotExist)
			vldr.AddFatal(ec(err, code.E001(specStr)))
		default:
			vldr.AddFatal(ec(err, code.E007(specStr)))
		}
		return nil, err
	}
	// validate root inventory
	invBytes, err := ReadAll(ctx, fsys, path.Join(dir, inventoryFile))
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			vldr.AddFatal(err, ec(err, code.E063(specStr)))
		default:
			vldr.AddFatal(err)
		}
		return nil, err
	}
	inv, invValidation := imp.ValidateInventoryBytes(invBytes)
	vldr.PrefixAdd("root inventory.json", invValidation)
	if err := invValidation.Err(); err != nil {
		return nil, err
	}
	if err := ValidateInventorySidecar(ctx, inv, fsys, dir); err != nil {
		switch {
		case errors.Is(err, ErrInventorySidecarContents):
			vldr.AddFatal(ec(err, code.E061(specStr)))
		default:
			vldr.AddFatal(ec(err, code.E060(specStr)))
		}
	}
	vldr.PrefixAdd("extensions directory", validateExtensionsDir(ctx, imp.spec, fsys, dir))
	if err := vldr.AddInventoryDigests(inv); err != nil {
		vldr.AddFatal(err)
	}
	vldr.PrefixAdd("root contents", validateRootState(imp.spec, state))
	if err := vldr.Err(); err != nil {
		return nil, err
	}
	// FIXME: shouldn't be initializing an object here.
	return &Object{
		fs:        fsys,
		path:      dir,
		inventory: inv,
		ocfl:      imp,
	}, nil
}

func (imp *ocflV1) ValidateObjectVersion(ctx context.Context, obj *Object, vnum VNum, verInv Inventory, prevInv Inventory, vldr *ObjectValidation) error {
	fsys := obj.FS()
	vnumStr := vnum.String()
	fullVerDir := path.Join(obj.Path(), vnumStr) // version directory path relative to FS
	specStr := string(imp.spec)
	rootInv := obj.Inventory() // headInv is assumed to be valid
	vDirEntries, err := fsys.ReadDir(ctx, fullVerDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		// can't read version directory for some reason, but not because it
		// doesn't exist.
		vldr.AddFatal(err)
		return err
	}
	vdirState := parseVersionDirState(vDirEntries)
	for _, f := range vdirState.extraFiles {
		err := fmt.Errorf(`unexpected file in %s: %s`, vnum, f)
		vldr.AddFatal(ec(err, code.E015(specStr)))
	}
	if !vdirState.hasInventory {
		err := fmt.Errorf("missing %s/inventory.json", vnumStr)
		vldr.AddWarn(ec(err, code.W010(specStr)))
	}
	if verInv != nil {
		verInvValidation := verInv.Validate()
		vldr.PrefixAdd(vnumStr+"/inventory.json", verInvValidation)
		if err := ValidateInventorySidecar(ctx, verInv, fsys, fullVerDir); err != nil {
			err := fmt.Errorf("%s/inventory.json: %w", vnumStr, err)
			switch {
			case errors.Is(err, ErrInventorySidecarContents):
				vldr.AddFatal(ec(err, code.E061(specStr)))
			default:
				vldr.AddFatal(ec(err, code.E060(specStr)))
			}
		}
		if prevInv != nil && verInv.Spec().Cmp(prevInv.Spec()) < 0 {
			err := fmt.Errorf("%s/inventory.json uses an older OCFL specification than than the previous version", vnum)
			vldr.AddFatal(ec(err, code.E103(specStr)))
		}
		if verInv.Head() != vnum {
			err := fmt.Errorf("%s/inventory.json: 'head' does not matchs its directory", vnum)
			vldr.AddFatal(ec(err, code.E040(specStr)))
		}
		if verInv.Digest() != rootInv.Digest() {
			imp.compareVersionInventory(obj, vnum, verInv, vldr)
			if verInv.Digest() != rootInv.Digest() {
				if err := vldr.AddInventoryDigests(verInv); err != nil {
					err = fmt.Errorf("%s/inventory.json digests are inconsistent with other inventories: %w", vnum, err)
					vldr.AddFatal(ec(err, code.E066(specStr)))
				}
			}
		}
	}
	cdName := rootInv.ContentDirectory()
	for _, d := range vdirState.dirs {
		// the only directory in the version directory SHOULD be the content directory
		if d != cdName {
			err := fmt.Errorf(`extra directory in %s: %s`, vnum, d)
			vldr.AddWarn(ec(err, code.W002(specStr)))
			continue
		}
		// add version content files to validation state
		var added int
		fullVerContDir := path.Join(fullVerDir, cdName)
		contentFiles, filesErrFn := WalkFiles(ctx, fsys, fullVerContDir)
		for contentFile := range contentFiles {
			// convert from path relative to version content directory to path
			// relative to the object
			vldr.AddExistingContent(path.Join(vnumStr, cdName, contentFile.Path))
			added++
		}
		if err := filesErrFn(); err != nil {
			vldr.AddFatal(err)
			return err
		}
		if added == 0 {
			// content directory exists but it's empty
			err := fmt.Errorf("content directory (%s) is empty directory", fullVerContDir)
			vldr.AddFatal(ec(err, code.E016(specStr)))
		}
	}
	return nil
}

func (imp *ocflV1) ValidateObjectContent(ctx context.Context, obj *Object, v *ObjectValidation) error {
	specStr := string(imp.spec)
	newVld := &Validation{}
	for name := range v.MissingContent() {
		err := fmt.Errorf("missing content: %s", name)
		newVld.AddFatal(ec(err, code.E092(specStr)))
	}
	for name := range v.UnexpectedContent() {
		err := fmt.Errorf("unexpected content: %s", name)
		newVld.AddFatal(ec(err, code.E023(specStr)))
	}
	if !v.SkipDigests() {
		alg := obj.Inventory().DigestAlgorithm()
		digests := v.ExistingContentDigests(obj.FS(), obj.Path(), alg)
		numgos := v.DigestConcurrency()
		registry := v.ValidationAlgorithms()
		for err := range digests.ValidateBatch(ctx, registry, numgos) {
			var digestErr *digest.DigestError
			switch {
			case errors.As(err, &digestErr):
				newVld.AddFatal(ec(digestErr, code.E093(specStr)))
			default:
				newVld.AddFatal(err)
			}
		}
	}
	v.Add(newVld)
	return newVld.Err()
}

func (imp ocflV1) compareVersionInventory(obj *Object, dirNum VNum, verInv Inventory, vldr *ObjectValidation) {
	rootInv := obj.Inventory()
	specStr := string(imp.spec)
	if verInv.Head() == rootInv.Head() && verInv.Digest() != rootInv.Digest() {
		err := fmt.Errorf("%s/inventor.json is not the same as the root inventory: digests don't match", dirNum)
		vldr.AddFatal(ec(err, code.E064(specStr)))
	}
	if verInv.ID() != rootInv.ID() {
		err := fmt.Errorf("%s/inventory.json: 'id' doesn't match value in root inventory", dirNum)
		vldr.AddFatal(ec(err, code.E037(specStr)))
	}
	if verInv.ContentDirectory() != rootInv.ContentDirectory() {
		err := fmt.Errorf("%s/inventory.json: 'contentDirectory' doesn't match value in root inventory", dirNum)
		vldr.AddFatal(ec(err, code.E019(specStr)))
	}
	// check that all version blocks in the version inventory
	// match version blocks in the root inventory
	for _, v := range verInv.Head().Lineage() {
		thisVersion := verInv.Version(v.Num())
		rootVersion := rootInv.Version(v.Num())
		if rootVersion == nil {
			err := fmt.Errorf("root inventory.json has missing version: %s", v)
			vldr.AddFatal(ec(err, code.E046(specStr)))
			continue
		}
		thisVerState := logicalState{
			state:    thisVersion.State(),
			manifest: verInv.Manifest(),
		}
		rootVerState := logicalState{
			state:    rootVersion.State(),
			manifest: rootInv.Manifest(),
		}
		if !thisVerState.Eq(rootVerState) {
			err := fmt.Errorf("%s/inventory.json has different logical state in its %s version block than the root inventory.json", dirNum, v)
			vldr.AddFatal(ec(err, code.E066(specStr)))
		}
		if thisVersion.Message() != rootVersion.Message() {
			err := fmt.Errorf("%s/inventory.json has different 'message' in its %s version block than the root inventory.json", dirNum, v)
			vldr.AddWarn(ec(err, code.W011(specStr)))
		}
		if !reflect.DeepEqual(thisVersion.User(), rootVersion.User()) {
			err := fmt.Errorf("%s/inventory.json has different 'user' in its %s version block than the root inventory.json", dirNum, v)
			vldr.AddWarn(ec(err, code.W011(specStr)))
		}
		if thisVersion.Created() != rootVersion.Created() {
			err := fmt.Errorf("%s/inventory.json has different 'created' in its %s version block than the root inventory.json", dirNum, v)
			vldr.AddWarn(ec(err, code.W011(specStr)))
		}
	}
}

type inventoryV1 struct {
	raw        rawInventory
	jsonDigest string
}

var _ Inventory = (*inventoryV1)(nil)

func (inv *inventoryV1) ContentDirectory() string {
	if c := inv.raw.ContentDirectory; c != "" {
		return c
	}
	return "content"
}

func (inv *inventoryV1) Digest() string { return inv.jsonDigest }

func (inv *inventoryV1) DigestAlgorithm() digest.Algorithm {
	// DigestAlgorithm should be sha512 or sha256
	switch inv.raw.DigestAlgorithm {
	case digest.SHA256.ID():
		return digest.SHA256
	case digest.SHA512.ID():
		return digest.SHA512
	default:
		return nil
	}
}

func (inv *inventoryV1) FixityAlgorithms() []string {
	if len(inv.raw.Fixity) < 1 {
		return nil
	}
	algs := make([]string, 0, len(inv.raw.Fixity))
	for alg := range inv.raw.Fixity {
		algs = append(algs, alg)
	}
	return algs
}

func (inv *inventoryV1) GetFixity(digest string) digest.Set {
	return inv.raw.getFixity(digest)
}

func (inv *inventoryV1) Head() VNum {
	return inv.raw.Head
}

func (inv *inventoryV1) ID() string {
	return inv.raw.ID
}

func (inv *inventoryV1) Manifest() DigestMap {
	return inv.raw.Manifest
}

func (inv *inventoryV1) Spec() Spec {
	return inv.raw.Type.Spec
}

func (inv *inventoryV1) Validate() *Validation {
	// if inv.raw.jsonDigest == "" {
	// 	err := errors.New("inventory was not initialized correctly: missing file digest value")
	// 	v := &Validation{}
	// 	v.AddFatal(err)
	// 	return v
	// }
	switch inv.Spec() {
	case Spec1_0:
		return validateInventoryV1(inv, Spec1_0)
	case Spec1_1:
		return validateInventoryV1(inv, Spec1_1)
	default:
		err := errors.New("OCFL v1.x inventory doesn't missing spec version")
		v := &Validation{}
		v.AddFatal(err)
		return v
	}
}

func (inv *inventoryV1) Version(i int) ObjectVersion {
	v := inv.raw.version(i)
	if v == nil {
		return nil
	}
	return &inventoryVersion{raw: v}
}

func (inv *inventoryV1) setJsonDigest(raw []byte) error {
	digester, err := digest.DefaultRegistry().NewDigester(inv.raw.DigestAlgorithm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(digester, bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("digesting inventory: %w", err)
	}
	inv.jsonDigest = digester.String()
	return nil
}

type inventoryVersion struct {
	raw *rawInventoryVersion
}

func (v *inventoryVersion) State() DigestMap   { return v.raw.State }
func (v *inventoryVersion) Message() string    { return v.raw.Message }
func (v *inventoryVersion) Created() time.Time { return v.raw.Created }
func (v *inventoryVersion) User() *User        { return v.raw.User }

func validateInventoryV1(inv *inventoryV1, spec Spec) *Validation {
	v := &Validation{}
	if inv.raw.Type.Empty() {
		err := errors.New("missing required field: 'type'")
		v.AddFatal(err)
	}
	if inv.raw.Type.Spec != spec {
		err := fmt.Errorf("inventory declares v%s, not v%s", inv.raw.Type.Spec, spec)
		v.AddFatal(err)
	}
	specStr := string(spec)
	if inv.raw.ID == "" {
		err := errors.New("missing required field: 'id'")
		v.AddFatal(verr(err, code.E036(specStr)))
	}
	if inv.raw.Head.IsZero() {
		err := errors.New("missing required field: 'head'")
		v.AddFatal(verr(err, code.E036(specStr)))
	}
	if inv.raw.Manifest == nil {
		err := errors.New("missing required field 'manifest'")
		v.AddFatal(verr(err, code.E041(specStr)))
	}
	if inv.raw.Versions == nil {
		err := errors.New("missing required field: 'versions'")
		v.AddFatal(verr(err, code.E041(specStr)))
	}
	if u, err := url.ParseRequestURI(inv.raw.ID); err != nil || u.Scheme == "" {
		err := fmt.Errorf(`object ID is not a URI: %q`, inv.raw.ID)
		v.AddWarn(verr(err, code.W005(specStr)))
	}
	switch inv.raw.DigestAlgorithm {
	case digest.SHA512.ID():
		break
	case digest.SHA256.ID():
		err := fmt.Errorf(`'digestAlgorithm' is %q`, digest.SHA256.ID())
		v.AddWarn(verr(err, code.W004(specStr)))
	default:
		err := fmt.Errorf(`'digestAlgorithm' is not %q or %q`, digest.SHA512.ID(), digest.SHA256.ID())
		v.AddFatal(verr(err, code.E025(specStr)))
	}
	if err := inv.raw.Head.Valid(); err != nil {
		err = fmt.Errorf("head is invalid: %w", err)
		v.AddFatal(verr(err, code.E011(specStr)))
	}
	if strings.Contains(inv.raw.ContentDirectory, "/") {
		err := errors.New("contentDirectory contains '/'")
		v.AddFatal(verr(err, code.E017(specStr)))
	}
	if inv.raw.ContentDirectory == "." || inv.raw.ContentDirectory == ".." {
		err := errors.New("contentDirectory is '.' or '..'")
		v.AddFatal(verr(err, code.E017(specStr)))
	}
	if inv.raw.Manifest != nil {
		err := inv.raw.Manifest.Valid()
		if err != nil {
			var dcErr *MapDigestConflictErr
			var pcErr *MapPathConflictErr
			var piErr *MapPathInvalidErr
			if errors.As(err, &dcErr) {
				err = verr(err, code.E096(specStr))
			} else if errors.As(err, &pcErr) {
				err = verr(err, code.E101(specStr))
			} else if errors.As(err, &piErr) {
				err = verr(err, code.E099(specStr))
			}
			v.AddFatal(err)
		}
		// check that each manifest entry is used in at least one state
		for _, digest := range inv.raw.Manifest.Digests() {
			var found bool
			for _, version := range inv.raw.Versions {
				if version == nil {
					continue
				}
				if len(version.State[digest]) > 0 {
					found = true
					break
				}
			}
			if !found {
				err := fmt.Errorf("digest in manifest not used in version state: %s", digest)
				v.AddFatal(verr(err, code.E107(specStr)))
			}
		}
	}
	// version names
	var versionNums VNums = inv.raw.vnums()
	if err := versionNums.Valid(); err != nil {
		if errors.Is(err, ErrVerEmpty) {
			err = verr(err, code.E008(specStr))
		} else if errors.Is(err, ErrVNumMissing) {
			err = verr(err, code.E010(specStr))
		} else if errors.Is(err, ErrVNumPadding) {
			err = verr(err, code.E012(specStr))
		}
		v.AddFatal(err)
	}
	if versionNums.Head() != inv.raw.Head {
		err := fmt.Errorf(`version head not most recent version: %s`, inv.raw.Head)
		v.AddFatal(verr(err, code.E040(specStr)))
	}
	// version state
	for vname, ver := range inv.raw.Versions {
		if ver == nil {
			err := fmt.Errorf(`missing required version block for %q`, vname)
			v.AddFatal(verr(err, code.E048(specStr)))
			continue
		}
		if ver.Created.IsZero() {
			err := fmt.Errorf(`version %s missing required field: 'created'`, vname)
			v.AddFatal(verr(err, code.E048(specStr)))
		}
		if ver.Message == "" {
			err := fmt.Errorf("version %s missing recommended field: 'message'", vname)
			v.AddWarn(verr(err, code.W007(specStr)))
		}
		if ver.User == nil {
			err := fmt.Errorf("version %s missing recommended field: 'user'", vname)
			v.AddWarn(verr(err, code.W007(specStr)))
		}
		if ver.User != nil {
			if ver.User.Name == "" {
				err := fmt.Errorf("version %s user missing required field: 'name'", vname)
				v.AddFatal(verr(err, code.E054(specStr)))
			}
			if ver.User.Address == "" {
				err := fmt.Errorf("version %s user missing recommended field: 'address'", vname)
				v.AddWarn(verr(err, code.W008(specStr)))
			}
			if u, err := url.ParseRequestURI(ver.User.Address); err != nil || u.Scheme == "" {
				err := fmt.Errorf("version %s user address is not a URI", vname)
				v.AddWarn(verr(err, code.W009(specStr)))
			}
		}
		if ver.State == nil {
			err := fmt.Errorf(`version %s missing required field: 'state'`, vname)
			v.AddFatal(verr(err, code.E048(specStr)))
			continue
		}
		err := ver.State.Valid()
		if err != nil {
			var dcErr *MapDigestConflictErr
			var pcErr *MapPathConflictErr
			var piErr *MapPathInvalidErr
			if errors.As(err, &dcErr) {
				err = verr(err, code.E050(specStr))
			} else if errors.As(err, &pcErr) {
				err = verr(err, code.E095(specStr))
			} else if errors.As(err, &piErr) {
				err = verr(err, code.E052(specStr))
			}
			v.AddFatal(err)
		}
		// check that each state digest appears in manifest
		for _, digest := range ver.State.Digests() {
			if len(inv.raw.Manifest[digest]) == 0 {
				err := fmt.Errorf("digest in %s state not in manifest: %s", vname, digest)
				v.AddFatal(verr(err, code.E050(specStr)))
			}
		}
	}
	//fixity
	for _, fixity := range inv.raw.Fixity {
		err := fixity.Valid()
		if err != nil {
			var dcErr *MapDigestConflictErr
			var piErr *MapPathInvalidErr
			var pcErr *MapPathConflictErr
			if errors.As(err, &dcErr) {
				err = verr(err, code.E097(specStr))
			} else if errors.As(err, &piErr) {
				err = verr(err, code.E099(specStr))
			} else if errors.As(err, &pcErr) {
				err = verr(err, code.E101(specStr))
			}
			v.AddFatal(err)
		}
	}
	return v
}

func jsonMapGet[T any](m map[string]any, key string) (val T, exists bool, typeOK bool) {
	var anyVal any
	anyVal, exists = m[key]
	val, typeOK = anyVal.(T)
	delete(m, key)
	return
}

func convertJSONDigestMap(jsonMap map[string]any) (DigestMap, error) {
	m := DigestMap{}
	msg := "invalid json type: expected array of strings"
	for key, mapVal := range jsonMap {
		slice, isSlice := mapVal.([]any)
		if !isSlice {
			return nil, errors.New(msg)
		}
		m[key] = make([]string, len(slice))
		for i := range slice {
			strVal, isStr := slice[i].(string)
			if !isStr {
				return nil, errors.New(msg)
			}
			m[key][i] = strVal
		}
	}
	return m, nil
}

type copyContentOpts struct {
	Source      ContentSource
	DestFS      WriteFS
	DestRoot    string
	Manifest    DigestMap
	Concurrency int
}

// transfer dst/src names in files from srcFS to dstFS
func copyContent(ctx context.Context, c *copyContentOpts) error {
	if c.Source == nil {
		return errors.New("missing countent source")
	}
	conc := c.Concurrency
	if conc < 1 {
		conc = 1
	}
	grp, ctx := errgroup.WithContext(ctx)
	grp.SetLimit(conc)
	for dig, dstNames := range c.Manifest {
		srcFS, srcPath := c.Source.GetContent(dig)
		if srcFS == nil {
			return fmt.Errorf("content source doesn't provide %q", dig)
		}
		for _, dstName := range dstNames {
			srcPath := srcPath
			dstPath := path.Join(c.DestRoot, dstName)
			grp.Go(func() error {
				return Copy(ctx, c.DestFS, dstPath, srcFS, srcPath)
			})

		}
	}
	return grp.Wait()
}

// newContentMap returns a DigestMap that is a subset of the inventory
// manifest for the digests and paths of new content
func newContentMap(inv *rawInventory) (DigestMap, error) {
	pm := PathMap{}
	var err error
	inv.Manifest.EachPath(func(pth, dig string) bool {
		// ignore manifest entries from previous versions
		if !strings.HasPrefix(pth, inv.Head.String()+"/") {
			return true
		}
		if _, exists := pm[pth]; exists {
			err = fmt.Errorf("path duplicate in manifest: %q", pth)
			return false
		}
		pm[pth] = dig
		return true
	})
	if err != nil {
		return nil, err
	}
	return pm.DigestMapValid()
}

func ec(err error, code *validation.ValidationCode) error {
	if code == nil {
		return err
	}
	return &ValidationError{
		Err:            err,
		ValidationCode: *code,
	}
}

// writeInventory marshals the value pointed to by inv, writing the json to dir/inventory.json in
// fsys. The digest is calculated using alg and the inventory sidecar is also written to
// dir/inventory.alg
func writeInventory(ctx context.Context, fsys WriteFS, inv *inventoryV1, dirs ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	byts, err := json.Marshal(inv.raw)
	if err != nil {
		return fmt.Errorf("encoding inventory: %w", err)
	}
	if err := inv.setJsonDigest(byts); err != nil {
		return fmt.Errorf("generating inventory.json checksum: %w", err)
	}
	// write inventory.json and sidecar
	for _, dir := range dirs {
		invFile := path.Join(dir, inventoryFile)
		sideFile := invFile + "." + inv.raw.DigestAlgorithm
		sideContent := inv.jsonDigest + " " + inventoryFile + "\n"
		_, err = fsys.Write(ctx, invFile, bytes.NewReader(byts))
		if err != nil {
			return fmt.Errorf("write inventory failed: %w", err)
		}
		_, err = fsys.Write(ctx, sideFile, strings.NewReader(sideContent))
		if err != nil {
			return fmt.Errorf("write inventory sidecar failed: %w", err)
		}
	}
	return nil
}

// NextInventory ...
func buildInventory(prev Inventory, commit *Commit) (*inventoryV1, error) {
	if commit.Stage == nil {
		return nil, errors.New("commit is missing new version state")
	}
	if commit.Stage.DigestAlgorithm == nil {
		return nil, errors.New("commit has no digest algorithm")

	}
	if commit.Stage.State == nil {
		commit.Stage.State = DigestMap{}
	}
	rawInv := &rawInventory{
		ID:               commit.ID,
		DigestAlgorithm:  commit.Stage.DigestAlgorithm.ID(),
		ContentDirectory: contentDir,
	}
	switch {
	case prev != nil:
		prevInv, ok := prev.(*inventoryV1)
		if !ok {
			err := errors.New("inventory is not an OCFLv1 inventory")
			return nil, err
		}
		if rawInv.DigestAlgorithm != prev.DigestAlgorithm().ID() {
			return nil, fmt.Errorf("commit must use same digest algorithm as existing inventory (%s)", prev.DigestAlgorithm())
		}
		rawInv.ID = prev.ID()
		rawInv.ContentDirectory = prevInv.raw.ContentDirectory
		rawInv.Type = prevInv.raw.Type
		var err error
		rawInv.Head, err = prev.Head().Next()
		if err != nil {
			return nil, fmt.Errorf("existing inventory's version scheme doesn't support additional versions: %w", err)
		}
		if !commit.Spec.Empty() {
			// new inventory spec must be >= prev
			if commit.Spec.Cmp(prev.Spec()) < 0 {
				err = fmt.Errorf("new inventory's OCFL spec can't be lower than the existing inventory's (%s)", prev.Spec())
				return nil, err
			}
			rawInv.Type = commit.Spec.AsInvType()
		}
		if !commit.AllowUnchanged {
			lastV := prev.Version(0)
			if lastV.State().Eq(commit.Stage.State) {
				err := errors.New("version state unchanged")
				return nil, err
			}
		}

		// copy and normalize all digests in the inventory. If we don't do this
		// non-normalized digests in previous version states might cause
		// problems since the updated manifest/fixity will be normalized.
		rawInv.Manifest, err = prev.Manifest().Normalize()
		if err != nil {
			return nil, fmt.Errorf("in existing inventory manifest: %w", err)
		}
		versions := prev.Head().Lineage()
		rawInv.Versions = make(map[VNum]*rawInventoryVersion, len(versions))
		for _, vnum := range versions {
			prevVer := prev.Version(vnum.Num())
			newVer := &rawInventoryVersion{
				Created: prevVer.Created(),
				Message: prevVer.Message(),
			}
			newVer.State, err = prevVer.State().Normalize()
			if err != nil {
				return nil, fmt.Errorf("in existing inventory %s state: %w", vnum, err)
			}
			if prevVer.User() != nil {
				newVer.User = &User{
					Name:    prevVer.User().Name,
					Address: prevVer.User().Address,
				}
			}
			rawInv.Versions[vnum] = newVer
		}
		// transfer fixity
		rawInv.Fixity = make(map[string]DigestMap, len(prevInv.raw.Fixity))
		for alg, m := range prevInv.raw.Fixity {
			rawInv.Fixity[alg], err = m.Normalize()
			if err != nil {
				return nil, fmt.Errorf("in existing inventory %s fixity: %w", alg, err)
			}
		}
	default:
		// FIXME: how whould padding be set for new inventories?
		rawInv.Head = V(1, 0)
		rawInv.Manifest = DigestMap{}
		rawInv.Fixity = map[string]DigestMap{}
		rawInv.Versions = map[VNum]*rawInventoryVersion{}
		rawInv.Type = commit.Spec.AsInvType()
	}

	// add new version
	newVersion := &rawInventoryVersion{
		State:   commit.Stage.State,
		Created: commit.Created,
		Message: commit.Message,
		User:    &commit.User,
	}
	if newVersion.Created.IsZero() {
		newVersion.Created = time.Now()
	}
	newVersion.Created = newVersion.Created.Truncate(time.Second)
	rawInv.Versions[rawInv.Head] = newVersion

	// build new manifest and fixity entries
	newContentFunc := func(paths []string) []string {
		// apply user-specified path transform first
		if commit.ContentPathFunc != nil {
			paths = commit.ContentPathFunc(paths)
		}
		contDir := rawInv.ContentDirectory
		if contDir == "" {
			contDir = contentDir
		}
		for i, p := range paths {
			paths[i] = path.Join(rawInv.Head.String(), contDir, p)
		}
		return paths
	}
	for digest, logicPaths := range newVersion.State {
		if len(rawInv.Manifest[digest]) > 0 {
			// version content already exists in the manifest
			continue
		}
		rawInv.Manifest[digest] = newContentFunc(slices.Clone(logicPaths))
	}
	if commit.Stage.FixitySource != nil {
		for digest, contentPaths := range rawInv.Manifest {
			fixSet := commit.Stage.FixitySource.GetFixity(digest)
			if len(fixSet) < 1 {
				continue
			}
			for fixAlg, fixDigest := range fixSet {
				if rawInv.Fixity[fixAlg] == nil {
					rawInv.Fixity[fixAlg] = DigestMap{}
				}
				for _, cp := range contentPaths {
					fixPaths := rawInv.Fixity[fixAlg][fixDigest]
					if !slices.Contains(fixPaths, cp) {
						rawInv.Fixity[fixAlg][fixDigest] = append(fixPaths, cp)
					}
				}
			}
		}
	}
	inv := &inventoryV1{raw: *rawInv}
	// check that resulting inventory is valid
	if err := inv.Validate().Err(); err != nil {
		return nil, fmt.Errorf("generated inventory is not valid: %w", err)
	}
	return inv, nil
}

type versionDirState struct {
	hasInventory bool
	sidecarAlg   string
	extraFiles   []string
	dirs         []string
}

func parseVersionDirState(entries []fs.DirEntry) versionDirState {
	var info versionDirState
	for _, e := range entries {
		if e.Type().IsDir() {
			info.dirs = append(info.dirs, e.Name())
			continue
		}
		if e.Type().IsRegular() || e.Type() == fs.ModeIrregular {
			if e.Name() == inventoryFile {
				info.hasInventory = true
				continue
			}
			if strings.HasPrefix(e.Name(), inventoryFile+".") && info.sidecarAlg == "" {
				info.sidecarAlg = strings.TrimPrefix(e.Name(), inventoryFile+".")
				continue
			}
		}
		// unexpected files
		info.extraFiles = append(info.extraFiles, e.Name())
	}
	return info
}

type logicalState struct {
	manifest DigestMap
	state    DigestMap
}

func (a logicalState) Eq(b logicalState) bool {
	if a.state == nil || b.state == nil || a.manifest == nil || b.manifest == nil {
		return false
	}
	if !a.state.EachPath(func(name string, dig string) bool {
		otherDig := b.state.GetDigest(name)
		if otherDig == "" {
			return false
		}
		contentPaths := a.manifest[dig]
		otherPaths := b.manifest[otherDig]
		if len(contentPaths) != len(otherPaths) {
			return false
		}
		sort.Strings(contentPaths)
		sort.Strings(otherPaths)
		for i, p := range contentPaths {
			if otherPaths[i] != p {
				return false
			}
		}
		return true
	}) {
		return false
	}
	// make sure all logical paths in other state are also in state
	return b.state.EachPath(func(otherName string, _ string) bool {
		return a.state.GetDigest(otherName) != ""
	})
}

func validateRootState(spec Spec, state *ObjectState) *Validation {
	specStr := string(spec)
	v := &Validation{}
	for _, name := range state.Invalid {
		err := fmt.Errorf(`%w: %s`, ErrObjRootStructure, name)
		v.AddFatal(ec(err, code.E001(specStr)))
	}
	if !state.HasInventory() {
		err := fmt.Errorf(`root inventory.json: %w`, fs.ErrNotExist)
		v.AddFatal(ec(err, code.E063(specStr)))
	}
	if !state.HasSidecar() {
		err := fmt.Errorf(`root inventory.json sidecar: %w`, fs.ErrNotExist)
		v.AddFatal(ec(err, code.E058(specStr)))
	}
	err := state.VersionDirs.Valid()
	if err != nil {
		if errors.Is(err, ErrVerEmpty) {
			err = ec(err, code.E008(specStr))
		} else if errors.Is(err, ErrVNumPadding) {
			err = ec(err, code.E011(specStr))
		} else if errors.Is(err, ErrVNumMissing) {
			err = ec(err, code.E010(specStr))
		}
		v.AddFatal(err)
	}
	if err == nil && state.VersionDirs.Padding() > 0 {
		err := errors.New("version directory names are zero-padded")
		v.AddWarn(ec(err, code.W001(specStr)))
	}
	// if vdirHead := state.VersionDirs.Head().Num(); vdirHead > o.inv.Head.Num() {
	// 	err := errors.New("version directories don't reflect versions in inventory.json")
	// 	v.AddFatal(ec(err, codes.E046(ocflV)))
	// }
	return v
}

func validateExtensionsDir(ctx context.Context, spec Spec, fsys FS, objDir string) *Validation {
	specStr := string(spec)
	v := &Validation{}
	extDir := path.Join(objDir, extensionsDir)
	items, err := fsys.ReadDir(ctx, extDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		v.AddFatal(err)
		return v
	}
	for _, i := range items {
		if !i.IsDir() {
			err := fmt.Errorf(`invalid file: %s`, i.Name())
			v.AddFatal(ec(err, code.E067(specStr)))
			continue
		}
		_, err := extension.Get(i.Name())
		if err != nil {
			// unknow extension
			err := fmt.Errorf("%w: %s", err, i.Name())
			v.AddWarn(ec(err, code.W013(specStr)))
		}
	}
	return v
}
