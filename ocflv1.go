package ocfl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/ocflv1/codes"
)

var OCFLv1_0 = ocflV1(Spec1_0)
var OCFLv1_1 = ocflV1(Spec1_1)

// implementation of ocfl v1.x
type ocflV1 Spec

func (imp ocflV1) Spec() Spec {
	return Spec(imp)
}

func (spec ocflV1) ValidateInventoryBytes(raw []byte) (*RawInventory, *Validation) {
	specStr := string(spec)
	v := &Validation{}
	invMap := map[string]any{}
	if err := json.Unmarshal(raw, &invMap); err != nil {
		err = fmt.Errorf("decoding inventory json: %w", err)
		v.AddFatal(verr(err, codes.E033(specStr)))
		return nil, v
	}
	const requiredErrMsg = "required field is missing or has unexpected json value"
	const optionalErrMsg = "optional field has unexpected json value"
	id, exists, typeOK := jsonMapGet[string](invMap, `id`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'id'`)
		v.AddFatal(verr(err, codes.E036(specStr)))
	}
	typeStr, exists, typeOK := jsonMapGet[string](invMap, `type`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'type'`)
		v.AddFatal(verr(err, codes.E036(specStr)))
	}
	if typeStr != "" && typeStr != Spec(spec).AsInvType().String() {
		err := fmt.Errorf("invalid inventory type value: %q", typeStr)
		v.AddFatal(verr(err, codes.E038(specStr)))
	}
	digestAlg, exists, typeOK := jsonMapGet[string](invMap, `digestAlgorithm`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'digestAlgorithm'`)
		v.AddFatal(verr(err, codes.E036(specStr)))
	}
	if digestAlg != "" && digestAlg != digest.SHA512.ID() && digestAlg != digest.SHA256.ID() {
		err := fmt.Errorf("invalid digest algorithm: %q", digestAlg)
		v.AddFatal(verr(err, codes.E025(specStr)))
	}
	head, exists, typeOK := jsonMapGet[string](invMap, `head`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'head'`)
		v.AddFatal(verr(err, codes.E036(specStr)))
	}
	manifestVals, exists, typeOK := jsonMapGet[map[string]any](invMap, `manifest`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'manifest'`)
		v.AddFatal(verr(err, codes.E041(specStr)))
	}
	versionsVals, exists, typeOK := jsonMapGet[map[string]any](invMap, `versions`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'versions'`)
		v.AddFatal(verr(err, codes.E043(specStr)))
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
		v.AddFatal(verr(err, codes.E111(specStr)))
	}
	// any remaining values in invVals are invalid
	for extra := range invMap {
		err := fmt.Errorf("inventory json has unexpected field: %q", extra)
		v.AddFatal(err)
	}
	inv := &RawInventory{
		ID:               id,
		ContentDirectory: contentDirectory,
		DigestAlgorithm:  digestAlg,
		Fixity:           map[string]DigestMap{},
		Versions:         make(map[VNum]*RawInventoryVersion),
	}
	if err := inv.Type.UnmarshalText([]byte(typeStr)); err != nil {
		v.AddFatal(verr(err, codes.E038(specStr)))
	}
	if err := inv.Head.UnmarshalText([]byte(head)); err != nil {
		v.AddFatal(verr(err, codes.E040(specStr)))
	}
	var err error
	if inv.Manifest, err = convertJSONDigestMap(manifestVals); err != nil {
		err = fmt.Errorf("invalid manifest: %w", err)
		v.AddFatal(verr(err, codes.E092(specStr)))
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
			v.AddFatal(verr(err, codes.E046(specStr)))
			continue
		}
		versionErrPrefix := "version '" + vnumStr + "'"
		versionVals, typeOK = val.(map[string]any)
		if !typeOK {
			err := errors.New(versionErrPrefix + ": value is not a json object")
			v.AddFatal(verr(err, codes.E045(specStr)))
		}
		createdStr, exists, typeOK = jsonMapGet[string](versionVals, `created`)
		if !exists || !typeOK {
			err := fmt.Errorf("%s: %s: %s", versionErrPrefix, requiredErrMsg, `'created'`)
			v.AddFatal(verr(err, codes.E048(specStr)))
		}
		if createdStr != "" {
			if err := created.UnmarshalText([]byte(createdStr)); err != nil {
				err = fmt.Errorf("%s: created: %w", versionErrPrefix, err)
				v.AddFatal(verr(err, codes.E049(specStr)))
			}
		}
		stateVals, exists, typeOK = jsonMapGet[map[string]any](versionVals, `state`)
		if !exists || !typeOK {
			err := fmt.Errorf("%s: %s: %q", versionErrPrefix, requiredErrMsg, `state`)
			v.AddFatal(verr(err, codes.E048(specStr)))
		}
		// message is optional
		message, exists, typeOK = jsonMapGet[string](versionVals, `message`)
		if exists && !typeOK {
			err := fmt.Errorf("%s: %s: %q", versionErrPrefix, optionalErrMsg, `message`)
			v.AddFatal(verr(err, codes.E094(specStr)))
		}
		// user is optional
		userVals, exists, typeOK := jsonMapGet[map[string]any](versionVals, `user`)
		switch {
		case exists && !typeOK:
			err := fmt.Errorf("%s: %s: %q", versionErrPrefix, optionalErrMsg, `user`)
			v.AddFatal(verr(err, codes.E054(specStr)))
		case exists:
			var userName, userAddress string
			userName, exists, typeOK = jsonMapGet[string](userVals, `name`)
			if !exists || !typeOK {
				err := fmt.Errorf("%s: user: %s: %q", versionErrPrefix, requiredErrMsg, `name`)
				v.AddFatal(verr(err, codes.E054(specStr)))
			}
			// address is optional
			userAddress, exists, typeOK = jsonMapGet[string](userVals, `address`)
			if exists && !typeOK {
				err := fmt.Errorf("%s: user: %s: %q", versionErrPrefix, optionalErrMsg, `address`)
				v.AddFatal(verr(err, codes.E054(specStr)))
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
		inv.Versions[vnum] = &RawInventoryVersion{
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
			v.AddFatal(verr(err, codes.E057(specStr)))
			continue
		}
		digests, err := convertJSONDigestMap(digestVals)
		if err != nil {
			err = fmt.Errorf("%s: %w", fixityErrPrefix, err)
			v.AddFatal(verr(err, codes.E057(specStr)))
			continue
		}
		inv.Fixity[algStr] = digests
	}
	if err := inv.setJsonDigest(raw); err != nil {
		v.AddFatal(err)
	}
	v.Add(spec.ValidateInventory(inv))
	if v.Err() != nil {
		return nil, v
	}
	return inv, v
}

func (imp ocflV1) ValidateInventory(inv *RawInventory) *Validation {
	v := &Validation{}
	if inv.Type.Empty() {
		err := errors.New("missing required field: 'type'")
		v.AddFatal(err)
	}
	ocflV := string(imp)
	if inv.Type.Spec != imp.Spec() {
		err := fmt.Errorf("inventory declares v%s, not v%s", ocflV, ocflV)
		v.AddFatal(err)
	}
	if inv.ID == "" {
		err := errors.New("missing required field: 'id'")
		v.AddFatal(verr(err, codes.E036(ocflV)))
	}
	if inv.Head.IsZero() {
		err := errors.New("missing required field: 'head'")
		v.AddFatal(verr(err, codes.E036(ocflV)))
	}
	if inv.Manifest == nil {
		err := errors.New("missing required field 'manifest'")
		v.AddFatal(verr(err, codes.E041(ocflV)))
	}
	if inv.Versions == nil {
		err := errors.New("missing required field: 'versions'")
		v.AddFatal(verr(err, codes.E041(ocflV)))
	}
	if u, err := url.ParseRequestURI(inv.ID); err != nil || u.Scheme == "" {
		err := fmt.Errorf(`object ID is not a URI: %q`, inv.ID)
		v.AddWarn(verr(err, codes.W005(ocflV)))
	}
	switch inv.DigestAlgorithm {
	case digest.SHA512.ID():
		break
	case digest.SHA256.ID():
		err := fmt.Errorf(`'digestAlgorithm' is %q`, digest.SHA256.ID())
		v.AddWarn(verr(err, codes.W004(ocflV)))
	default:
		err := fmt.Errorf(`'digestAlgorithm' is not %q or %q`, digest.SHA512.ID(), digest.SHA256.ID())
		v.AddFatal(verr(err, codes.E025(ocflV)))
	}
	if err := inv.Head.Valid(); err != nil {
		err = fmt.Errorf("head is invalid: %w", err)
		v.AddFatal(verr(err, codes.E011(ocflV)))
	}
	if strings.Contains(inv.ContentDirectory, "/") {
		err := errors.New("contentDirectory contains '/'")
		v.AddFatal(verr(err, codes.E017(ocflV)))
	}
	if inv.ContentDirectory == "." || inv.ContentDirectory == ".." {
		err := errors.New("contentDirectory is '.' or '..'")
		v.AddFatal(verr(err, codes.E017(ocflV)))
	}
	if inv.Manifest != nil {
		err := inv.Manifest.Valid()
		if err != nil {
			var dcErr *MapDigestConflictErr
			var pcErr *MapPathConflictErr
			var piErr *MapPathInvalidErr
			if errors.As(err, &dcErr) {
				err = verr(err, codes.E096(ocflV))
			} else if errors.As(err, &pcErr) {
				err = verr(err, codes.E101(ocflV))
			} else if errors.As(err, &piErr) {
				err = verr(err, codes.E099(ocflV))
			}
			v.AddFatal(err)
		}
		// check that each manifest entry is used in at least one state
		for _, digest := range inv.Manifest.Digests() {
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
				v.AddFatal(verr(err, codes.E107(ocflV)))
			}
		}
	}
	// version names
	var versionNums VNums = inv.VNums()
	if err := versionNums.Valid(); err != nil {
		if errors.Is(err, ErrVerEmpty) {
			err = verr(err, codes.E008(ocflV))
		} else if errors.Is(err, ErrVNumMissing) {
			err = verr(err, codes.E010(ocflV))
		} else if errors.Is(err, ErrVNumPadding) {
			err = verr(err, codes.E012(ocflV))
		}
		v.AddFatal(err)
	}
	if versionNums.Head() != inv.Head {
		err := fmt.Errorf(`version head not most recent version: %s`, inv.Head)
		v.AddFatal(verr(err, codes.E040(ocflV)))
	}
	// version state
	for vname, ver := range inv.Versions {
		if ver == nil {
			err := fmt.Errorf(`missing required version block for %q`, vname)
			v.AddFatal(verr(err, codes.E048(ocflV)))
			continue
		}
		if ver.Created.IsZero() {
			err := fmt.Errorf(`version %s missing required field: 'created'`, vname)
			v.AddFatal(verr(err, codes.E048(ocflV)))
		}
		if ver.Message == "" {
			err := fmt.Errorf("version %s missing recommended field: 'message'", vname)
			v.AddWarn(verr(err, codes.W007(ocflV)))
		}
		if ver.User == nil {
			err := fmt.Errorf("version %s missing recommended field: 'user'", vname)
			v.AddWarn(verr(err, codes.W007(ocflV)))
		}
		if ver.User != nil {
			if ver.User.Name == "" {
				err := fmt.Errorf("version %s user missing required field: 'name'", vname)
				v.AddFatal(verr(err, codes.E054(ocflV)))
			}
			if ver.User.Address == "" {
				err := fmt.Errorf("version %s user missing recommended field: 'address'", vname)
				v.AddWarn(verr(err, codes.W008(ocflV)))
			}
			if u, err := url.ParseRequestURI(ver.User.Address); err != nil || u.Scheme == "" {
				err := fmt.Errorf("version %s user address is not a URI", vname)
				v.AddWarn(verr(err, codes.W009(ocflV)))
			}
		}
		if ver.State == nil {
			err := fmt.Errorf(`version %s missing required field: 'state'`, vname)
			v.AddFatal(verr(err, codes.E048(ocflV)))
			continue
		}
		err := ver.State.Valid()
		if err != nil {
			var dcErr *MapDigestConflictErr
			var pcErr *MapPathConflictErr
			var piErr *MapPathInvalidErr
			if errors.As(err, &dcErr) {
				err = verr(err, codes.E050(ocflV))
			} else if errors.As(err, &pcErr) {
				err = verr(err, codes.E095(ocflV))
			} else if errors.As(err, &piErr) {
				err = verr(err, codes.E052(ocflV))
			}
			v.AddFatal(err)
		}
		// check that each state digest appears in manifest
		for _, digest := range ver.State.Digests() {
			if len(inv.Manifest[digest]) == 0 {
				err := fmt.Errorf("digest in %s state not in manifest: %s", vname, digest)
				v.AddFatal(verr(err, codes.E050(ocflV)))
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
				err = verr(err, codes.E097(ocflV))
			} else if errors.As(err, &piErr) {
				err = verr(err, codes.E099(ocflV))
			} else if errors.As(err, &pcErr) {
				err = verr(err, codes.E101(ocflV))
			}
			v.AddFatal(err)
		}
	}
	return v
}

func (imp *ocflV1) NewReadInventory(raw []byte) (*RawInventoryVersion, error) {
	return nil, errors.New("not implemented")
}

func (imp *ocflV1) NewReadObject(fsys FS, path string, inv ReadInventory) ReadObject {
	return nil
}
func (imp *ocflV1) Commit(ctx context.Context, obj ReadObject, commit *Commit) (ReadObject, error) {
	return nil, errors.New("not implemented")
}
func (imp *ocflV1) ValidateObjectRoot(ctx context.Context, fs FS, dir string, state *ObjectState, vldr *ObjectValidation) (ReadObject, error) {
	return nil, errors.New("not implemented")

}
func (imp *ocflV1) ValidateObjectVersion(ctx context.Context, obj ReadObject, vnum VNum, versionInv ReadInventory, prevInv ReadInventory, vldr *ObjectValidation) error {
	return errors.New("not implemented")

}
func (imp *ocflV1) ValidateObjectContent(ctx context.Context, obj ReadObject, vldr *ObjectValidation) error {
	return errors.New("not implemented")

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
