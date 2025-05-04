package ocfl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/extension"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/validation/code"
)

// ocflv1 is an implementation of ocfl v1.x
type ocflV1 struct {
	v1Spec Spec // "1.0" or "1.1"
}

var _ ocfl = (*ocflV1)(nil)

func (imp ocflV1) Spec() Spec {
	return Spec(imp.v1Spec)
}

func (imp ocflV1) ValidateInventory(inv *Inventory) *Validation {
	v := &Validation{}
	if inv.Type.Empty() {
		err := errors.New("missing required field: 'type'")
		v.AddFatal(err)
	}
	if inv.Type.Spec != imp.v1Spec {
		err := fmt.Errorf("inventory declares v%s, not v%s", inv.Type.Spec, imp.v1Spec)
		v.AddFatal(err)
	}
	specStr := string(imp.v1Spec)
	if inv.ID == "" {
		err := errors.New("missing required field: 'id'")
		v.AddFatal(verr(err, code.E036(specStr)))
	}
	if inv.Head.IsZero() {
		err := errors.New("missing required field: 'head'")
		v.AddFatal(verr(err, code.E036(specStr)))
	}
	if inv.Manifest == nil {
		err := errors.New("missing required field 'manifest'")
		v.AddFatal(verr(err, code.E041(specStr)))
	}
	if inv.Versions == nil {
		err := errors.New("missing required field: 'versions'")
		v.AddFatal(verr(err, code.E041(specStr)))
	}
	if u, err := url.ParseRequestURI(inv.ID); err != nil || u.Scheme == "" {
		err := fmt.Errorf(`object ID is not a URI: %q`, inv.ID)
		v.AddWarn(verr(err, code.W005(specStr)))
	}
	switch inv.DigestAlgorithm {
	case digest.SHA512.ID():
		break
	case digest.SHA256.ID():
		err := fmt.Errorf(`'digestAlgorithm' is %q`, digest.SHA256.ID())
		v.AddWarn(verr(err, code.W004(specStr)))
	default:
		err := fmt.Errorf(`'digestAlgorithm' is not %q or %q`, digest.SHA512.ID(), digest.SHA256.ID())
		v.AddFatal(verr(err, code.E025(specStr)))
	}
	if err := inv.Head.Valid(); err != nil {
		err = fmt.Errorf("head is invalid: %w", err)
		v.AddFatal(verr(err, code.E011(specStr)))
	}
	if strings.Contains(inv.ContentDirectory, "/") {
		err := errors.New("contentDirectory contains '/'")
		v.AddFatal(verr(err, code.E017(specStr)))
	}
	if inv.ContentDirectory == "." || inv.ContentDirectory == ".." {
		err := errors.New("contentDirectory is '.' or '..'")
		v.AddFatal(verr(err, code.E017(specStr)))
	}
	if inv.Manifest != nil {
		err := inv.Manifest.Valid()
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
		for digest := range inv.Manifest {
			var found bool
			for _, version := range inv.Versions {
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
	var versionNums VNums = inv.vnums()
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
	if versionNums.Head() != inv.Head {
		err := fmt.Errorf(`version head not most recent version: %s`, inv.Head)
		v.AddFatal(verr(err, code.E040(specStr)))
	}
	// version state
	for vname, ver := range inv.Versions {
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
		for digest := range ver.State {
			if len(inv.Manifest[digest]) == 0 {
				err := fmt.Errorf("digest in %s state not in manifest: %s", vname, digest)
				v.AddFatal(verr(err, code.E050(specStr)))
			}
		}
	}
	//fixity
	for _, fixity := range inv.Fixity {
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

func (imp ocflV1) ValidateInventoryBytes(raw []byte) (*Inventory, *Validation) {
	specStr := string(imp.v1Spec)
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
	if typeStr != "" && typeStr != Spec(imp.v1Spec).InventoryType().String() {
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
	inv := &Inventory{
		ID:               id,
		ContentDirectory: contentDirectory,
		DigestAlgorithm:  digestAlg,
		Fixity:           map[string]DigestMap{},
		Versions:         make(map[VNum]*InventoryVersion),
	}
	if err := inv.Type.UnmarshalText([]byte(typeStr)); err != nil {
		v.AddFatal(verr(err, code.E038(specStr)))
	}
	if err := inv.Head.UnmarshalText([]byte(head)); err != nil {
		v.AddFatal(verr(err, code.E040(specStr)))
	}
	var err error
	if inv.Manifest, err = convertJSONDigestMap(manifestVals); err != nil {
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
		inv.Versions[vnum] = &InventoryVersion{
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
		inv.Fixity[algStr] = digests
	}
	if err := inv.setJsonDigest(raw); err != nil {
		v.AddFatal(err)
	}
	v.Add(inv.Validate())
	if v.Err() != nil {
		return nil, v
	}
	return inv, v
}

func (imp ocflV1) newCommitPlan(ctx context.Context, obj *Object, commit *Commit) (*commitPlan, error) {
	newInv, err := imp.newInventoryV1(commit, obj.rootInventory)
	if err != nil {
		return nil, fmt.Errorf("building new inventory: %w", err)
	}
	// newContent is a subeset of the manifest with the new content to add
	newContent, err := newContentMap(newInv)
	if err != nil {
		return nil, err
	}
	// check that the stage includes all the new content
	for digest := range newContent {
		if !commit.Stage.HasContent(digest) {
			// FIXME short digest
			err := fmt.Errorf("no content for digest: %s", digest)
			return nil, err
		}
	}
	plan := &commitPlan{
		FS:            obj.FS(),
		Path:          obj.Path(),
		NewInventory:  newInv,
		PrevInventoy:  obj.rootInventory,
		NewContent:    newContent,
		ContentSource: commit.Stage,
	}
	return plan, nil
}

func (imp ocflV1) Commit(ctx context.Context, obj *Object, commit *Commit) error {
	plan, err := imp.newCommitPlan(ctx, obj, commit)
	if err != nil {
		return &CommitError{Err: err}
	}
	if err := plan.Run(ctx, commit.Logger); err != nil {
		return &CommitError{Err: err, Dirty: true}
	}
	obj.rootInventory = plan.NewInventory
	return nil
}

func (imp ocflV1) ValidateObjectRoot(ctx context.Context, vldr *ObjectValidation, state *ObjectState) error {
	// validate namaste
	specStr := string(imp.v1Spec)
	decl := Namaste{Type: NamasteTypeObject, Version: imp.v1Spec}
	name := path.Join(vldr.path(), decl.Name())
	err := ValidateNamaste(ctx, vldr.fs(), name)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			err = fmt.Errorf("%s: %w", name, ErrObjectNamasteNotExist)
			vldr.AddFatal(verr(err, code.E001(specStr)))
		default:
			vldr.AddFatal(verr(err, code.E007(specStr)))
		}
		return err
	}
	// validate root inventory
	invBytes, err := ocflfs.ReadAll(ctx, vldr.fs(), path.Join(vldr.path(), inventoryBase))
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			vldr.AddFatal(err, verr(err, code.E063(specStr)))
		default:
			vldr.AddFatal(err)
		}
		return err
	}
	inv, invValidation := imp.ValidateInventoryBytes(invBytes)
	vldr.PrefixAdd("root inventory.json", invValidation)
	if err := invValidation.Err(); err != nil {
		return err
	}
	if err := ValidateInventorySidecar(ctx, inv, vldr.fs(), vldr.path()); err != nil {
		switch {
		case errors.Is(err, ErrInventorySidecarContents):
			vldr.AddFatal(verr(err, code.E061(specStr)))
		default:
			vldr.AddFatal(verr(err, code.E060(specStr)))
		}
	}
	vldr.PrefixAdd("extensions directory", validateExtensionsDir(ctx, imp.v1Spec, vldr.fs(), vldr.path()))
	if err := vldr.addInventory(inv, true); err != nil {
		vldr.AddFatal(err)
	}
	vldr.PrefixAdd("root contents", validateRootState(imp.v1Spec, state))
	if err := vldr.Err(); err != nil {
		return err
	}
	return nil
}

func (imp ocflV1) ValidateObjectVersion(ctx context.Context, vldr *ObjectValidation, vnum VNum, verInv *Inventory, prevInv *Inventory) error {
	fsys := vldr.fs()
	vnumStr := vnum.String()
	fullVerDir := path.Join(vldr.path(), vnumStr) // version directory path relative to FS
	specStr := string(imp.v1Spec)
	rootInv := vldr.obj.rootInventory // rootInv is assumed to be valid
	vDirEntries, err := ocflfs.ReadDir(ctx, fsys, fullVerDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		// can't read version directory for some reason, but not because it
		// doesn't exist.
		vldr.AddFatal(err)
		return err
	}
	vdirState := parseVersionDirState(vDirEntries)
	for _, f := range vdirState.extraFiles {
		err := fmt.Errorf(`unexpected file in %s: %s`, vnum, f)
		vldr.AddFatal(verr(err, code.E015(specStr)))
	}
	if !vdirState.hasInventory {
		err := fmt.Errorf("missing %s/inventory.json", vnumStr)
		vldr.AddWarn(verr(err, code.W010(specStr)))
	}
	if verInv != nil {
		verInvValidation := imp.ValidateInventory(verInv)
		vldr.PrefixAdd(vnumStr+"/inventory.json", verInvValidation)
		if err := ValidateInventorySidecar(ctx, verInv, fsys, fullVerDir); err != nil {
			err := fmt.Errorf("%s/inventory.json: %w", vnumStr, err)
			switch {
			case errors.Is(err, ErrInventorySidecarContents):
				vldr.AddFatal(verr(err, code.E061(specStr)))
			default:
				vldr.AddFatal(verr(err, code.E060(specStr)))
			}
		}
		if prevInv != nil && verInv.Type.Cmp(prevInv.Type.Spec) < 0 {
			err := fmt.Errorf("%s/inventory.json uses an older OCFL specification than than the previous version", vnum)
			vldr.AddFatal(verr(err, code.E103(specStr)))
		}
		if verInv.Head != vnum {
			err := fmt.Errorf("%s/inventory.json: 'head' does not matchs its directory", vnum)
			vldr.AddFatal(verr(err, code.E040(specStr)))
		}
		if verInv.Digest() != rootInv.Digest() {
			imp.compareVersionInventory(vldr.obj, vnum, verInv, vldr)
			if verInv.Digest() != rootInv.Digest() {
				if err := vldr.addInventory(verInv, false); err != nil {
					err = fmt.Errorf("%s/inventory.json digests are inconsistent with other inventories: %w", vnum, err)
					vldr.AddFatal(verr(err, code.E066(specStr)))
				}
			}
		}
	}
	cdName := vldr.obj.ContentDirectory()
	for _, d := range vdirState.dirs {
		// the only directory in the version directory SHOULD be the content directory
		if d != cdName {
			err := fmt.Errorf(`extra directory in %s: %s`, vnum, d)
			vldr.AddWarn(verr(err, code.W002(specStr)))
			continue
		}
		// add version content files to validation state
		var added int
		fullVerContDir := path.Join(fullVerDir, cdName)
		for contentFile, err := range ocflfs.WalkFiles(ctx, fsys, fullVerContDir) {
			if err != nil {
				vldr.AddFatal(err)
				return err
			}
			// convert from path relative to version content directory to path
			// relative to the object
			vldr.addExistingContent(path.Join(vnumStr, cdName, contentFile.Path))
			added++
		}
		if added == 0 {
			// content directory exists but it's empty
			err := fmt.Errorf("content directory (%s) is empty directory", fullVerContDir)
			vldr.AddFatal(verr(err, code.E016(specStr)))
		}
	}
	return nil
}

func (imp ocflV1) ValidateObjectContent(ctx context.Context, v *ObjectValidation) error {
	specStr := string(imp.v1Spec)
	newVld := &Validation{}
	for name := range v.missingContent() {
		err := fmt.Errorf("missing content: %s", name)
		newVld.AddFatal(verr(err, code.E092(specStr)))
	}
	for name := range v.unexpectedContent() {
		err := fmt.Errorf("unexpected content: %s", name)
		newVld.AddFatal(verr(err, code.E023(specStr)))
	}
	if !v.SkipDigests() {
		alg := v.obj.DigestAlgorithm()
		digests := v.existingContentDigests(v.fs(), v.path(), alg)
		numgos := v.DigestConcurrency()
		registry := v.ValidationAlgorithms()
		for err := range digest.ValidateFilesBatch(ctx, digests, registry, numgos) {
			var digestErr *digest.DigestError
			switch {
			case errors.As(err, &digestErr):
				newVld.AddFatal(verr(digestErr, code.E093(specStr)))
			default:
				newVld.AddFatal(err)
			}
		}
	}
	v.Add(newVld)
	return newVld.Err()
}

func (imp ocflV1) compareVersionInventory(obj *Object, dirNum VNum, verInv *Inventory, vldr *ObjectValidation) {
	rootInv := obj.rootInventory
	specStr := string(imp.v1Spec)
	if verInv.Head == rootInv.Head && verInv.Digest() != rootInv.Digest() {
		err := fmt.Errorf("%s/inventor.json is not the same as the root inventory: digests don't match", dirNum)
		vldr.AddFatal(verr(err, code.E064(specStr)))
	}
	if verInv.ID != rootInv.ID {
		err := fmt.Errorf("%s/inventory.json: 'id' doesn't match value in root inventory", dirNum)
		vldr.AddFatal(verr(err, code.E037(specStr)))
	}
	if verInv.ContentDirectory != rootInv.ContentDirectory {
		err := fmt.Errorf("%s/inventory.json: 'contentDirectory' doesn't match value in root inventory", dirNum)
		vldr.AddFatal(verr(err, code.E019(specStr)))
	}
	// check that all version blocks in the version inventory
	// match version blocks in the root inventory
	for _, v := range verInv.Head.Lineage() {
		thisVersion := verInv.Versions[v]
		rootVersion := rootInv.Versions[v]
		if rootVersion == nil {
			err := fmt.Errorf("root inventory.json has missing version: %s", v)
			vldr.AddFatal(verr(err, code.E046(specStr)))
			continue
		}
		thisVerState := logicalState{
			state:    thisVersion.State,
			manifest: verInv.Manifest,
		}
		rootVerState := logicalState{
			state:    rootVersion.State,
			manifest: rootInv.Manifest,
		}
		if !thisVerState.Eq(rootVerState) {
			err := fmt.Errorf("%s/inventory.json has different logical state in its %s version block than the root inventory.json", dirNum, v)
			vldr.AddFatal(verr(err, code.E066(specStr)))
		}
		if thisVersion.Message != rootVersion.Message {
			err := fmt.Errorf("%s/inventory.json has different 'message' in its %s version block than the root inventory.json", dirNum, v)
			vldr.AddWarn(verr(err, code.W011(specStr)))
		}
		if !reflect.DeepEqual(thisVersion.User, rootVersion.User) {
			err := fmt.Errorf("%s/inventory.json has different 'user' in its %s version block than the root inventory.json", dirNum, v)
			vldr.AddWarn(verr(err, code.W011(specStr)))
		}
		if thisVersion.Created != rootVersion.Created {
			err := fmt.Errorf("%s/inventory.json has different 'created' in its %s version block than the root inventory.json", dirNum, v)
			vldr.AddWarn(verr(err, code.W011(specStr)))
		}
	}
}

// build a new inventoryV1 from a commit and an optional previous inventory
func (imp ocflV1) newInventoryV1(commit *Commit, prev *Inventory) (*Inventory, error) {
	if commit.Stage == nil {
		return nil, errors.New("commit is missing new version state")
	}
	if commit.Stage.DigestAlgorithm == nil {
		return nil, errors.New("commit has no digest algorithm")

	}
	if commit.Stage.State == nil {
		commit.Stage.State = DigestMap{}
	}
	id := commit.ID
	if prev != nil {
		if !commit.AllowUnchanged {
			lastV := prev.Versions[prev.Head]
			if lastV != nil && lastV.State.Eq(commit.Stage.State) {
				err := errors.New("version state unchanged")
				return nil, err
			}
		}
		id = prev.ID
	}
	newInv, err := NewInventoryBuilder(prev).
		ID(id).
		ContentPathFunc(commit.ContentPathFunc).
		FixitySource(commit.Stage).
		Spec(commit.Spec).
		AddVersion(
			commit.Stage.State,
			commit.Stage.DigestAlgorithm,
			commit.Created,
			commit.Message,
			&commit.User,
		).Finalize()

	if err != nil {
		return nil, err
	}
	return newInv, nil
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

// writeInventory marshals the value pointed to by inv, writing the json to dir/inventory.json in
// fsys. The digest is calculated using alg and the inventory sidecar is also written to
// dir/inventory.alg
func writeInventory(ctx context.Context, fsys ocflfs.FS, inv *Inventory, dirs ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	byts, err := json.Marshal(inv)
	if err != nil {
		return fmt.Errorf("encoding inventory: %w", err)
	}
	if err := inv.setJsonDigest(byts); err != nil {
		return fmt.Errorf("generating inventory.json checksum: %w", err)
	}
	// write inventory.json and sidecar
	for _, dir := range dirs {
		invFile := path.Join(dir, inventoryBase)
		sideFile := invFile + "." + inv.DigestAlgorithm
		sideContent := inv.jsonDigest + " " + inventoryBase + "\n"
		_, err = ocflfs.Write(ctx, fsys, invFile, bytes.NewReader(byts))
		if err != nil {
			return fmt.Errorf("write inventory failed: %w", err)
		}
		_, err = ocflfs.Write(ctx, fsys, sideFile, strings.NewReader(sideContent))
		if err != nil {
			return fmt.Errorf("write inventory sidecar failed: %w", err)
		}
	}
	return nil
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
			if e.Name() == inventoryBase {
				info.hasInventory = true
				continue
			}
			if strings.HasPrefix(e.Name(), inventoryBase+".") && info.sidecarAlg == "" {
				info.sidecarAlg = strings.TrimPrefix(e.Name(), inventoryBase+".")
				continue
			}
		}
		// unexpected files
		info.extraFiles = append(info.extraFiles, e.Name())
	}
	return info
}

// logicalState includes an inventory manifest and the version state,
// both of which are needed to map from logical path -> content paths.
type logicalState struct {
	manifest DigestMap
	state    DigestMap
}

func (a logicalState) Eq(b logicalState) bool {
	if a.state == nil || b.state == nil || a.manifest == nil || b.manifest == nil {
		return false
	}
	for name, dig := range a.state.Paths() {
		otherDig := b.state.DigestFor(name)
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
	}
	// make sure all logical paths in other state are also in state
	for otherName := range b.state.Paths() {
		if a.state.DigestFor(otherName) == "" {
			return false
		}
	}
	return true
}

func validateRootState(spec Spec, state *ObjectState) *Validation {
	specStr := string(spec)
	v := &Validation{}
	for _, name := range state.Invalid {
		err := fmt.Errorf(`%w: %s`, ErrObjRootStructure, name)
		v.AddFatal(verr(err, code.E001(specStr)))
	}
	if !state.HasInventory() {
		err := fmt.Errorf(`root inventory.json: %w`, fs.ErrNotExist)
		v.AddFatal(verr(err, code.E063(specStr)))
	}
	if !state.HasSidecar() {
		err := fmt.Errorf(`root inventory.json sidecar: %w`, fs.ErrNotExist)
		v.AddFatal(verr(err, code.E058(specStr)))
	}
	err := state.VersionDirs.Valid()
	if err != nil {
		if errors.Is(err, ErrVerEmpty) {
			err = verr(err, code.E008(specStr))
		} else if errors.Is(err, ErrVNumPadding) {
			err = verr(err, code.E011(specStr))
		} else if errors.Is(err, ErrVNumMissing) {
			err = verr(err, code.E010(specStr))
		}
		v.AddFatal(err)
	}
	if err == nil && state.VersionDirs.Padding() > 0 {
		err := errors.New("version directory names are zero-padded")
		v.AddWarn(verr(err, code.W001(specStr)))
	}
	// if vdirHead := state.VersionDirs.Head().Num(); vdirHead > o.inv.Head.Num() {
	// 	err := errors.New("version directories don't reflect versions in inventory.json")
	// 	v.AddFatal(verr(err, codes.E046(ocflV)))
	// }
	return v
}

func validateExtensionsDir(ctx context.Context, spec Spec, fsys ocflfs.FS, objDir string) *Validation {
	specStr := string(spec)
	v := &Validation{}
	extDir := path.Join(objDir, extensionsDir)
	items, err := ocflfs.ReadDir(ctx, fsys, extDir)
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
			v.AddFatal(verr(err, code.E067(specStr)))
			continue
		}
		_, err := extension.Get(i.Name())
		if err != nil {
			// unknow extension
			err := fmt.Errorf("%w: %s", err, i.Name())
			v.AddWarn(verr(err, code.W013(specStr)))
		}
	}
	return v
}
