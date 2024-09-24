package ocflv1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1/codes"
	"golang.org/x/exp/maps"
)

var (
	ErrVersionNotFound = errors.New("version not found in inventory")
)

// Inventory represents raw contents of an OCFL v1.x inventory.json file
type Inventory struct {
	ID               string                    `json:"id"`
	Type             ocfl.InvType              `json:"type"`
	DigestAlgorithm  string                    `json:"digestAlgorithm"`
	Head             ocfl.VNum                 `json:"head"`
	ContentDirectory string                    `json:"contentDirectory,omitempty"`
	Manifest         ocfl.DigestMap            `json:"manifest"`
	Versions         map[ocfl.VNum]*Version    `json:"versions"`
	Fixity           map[string]ocfl.DigestMap `json:"fixity,omitempty"`

	// digest of raw inventory using DigestAlgorithm, set during json
	// marshal/unmarshal
	jsonDigest string
}

// Version represents object version state and metadata
type Version struct {
	Created time.Time      `json:"created"`
	State   ocfl.DigestMap `json:"state"`
	Message string         `json:"message,omitempty"`
	User    *ocfl.User     `json:"user,omitempty"`
}

func (inv *Inventory) MarshalJSON() ([]byte, error) {
	type invAlias Inventory
	alias := (*invAlias)(inv)
	byts, err := json.Marshal(alias)
	if err != nil {
		return nil, err
	}
	digester := ocfl.NewDigester(inv.DigestAlgorithm)
	if digester == nil {
		return nil, fmt.Errorf("inventory digest algorithm %q: %w", inv.DigestAlgorithm, ocfl.ErrUnknownAlg)
	}
	if _, err := io.Copy(digester, bytes.NewReader(byts)); err != nil {
		return nil, err
	}
	inv.jsonDigest = digester.String()
	return byts, nil
}

// VNums returns a sorted slice of VNums corresponding to the keys in the
// inventory's 'versions' block.
func (inv Inventory) VNums() []ocfl.VNum {
	vnums := make([]ocfl.VNum, len(inv.Versions))
	i := 0
	for v := range inv.Versions {
		vnums[i] = v
		i++
	}
	sort.Sort(ocfl.VNums(vnums))
	return vnums
}

// Digest of Inventory's source json using the inventory digest. If the
// Inventory wasn't decoded using ValidateInventoryBytes
func (inv Inventory) Digest() string {
	return inv.jsonDigest
}

// ContentPath resolves the logical path from the version state with number v to
// a content path (i.e., a manifest path). The content path is relative to the
// object's root directory. If v is zero, the inventories head version is used.
func (inv Inventory) ContentPath(v int, logical string) (string, error) {
	ver := inv.Version(v)
	if ver == nil {
		return "", ErrVersionNotFound
	}
	sum := ver.State.GetDigest(logical)
	if sum == "" {
		return "", fmt.Errorf("no path: %s", logical)
	}
	paths := inv.Manifest[sum]
	if len(paths) == 0 {
		return "", fmt.Errorf("missing manifest entry for: %s", sum)
	}
	return paths[0], nil
}

// Version returns the version entry from the entry with number v. If v is 0,
// the head version is used. If no version entry exists, nil is returned
func (inv Inventory) Version(v int) *Version {
	if inv.Versions == nil {
		return nil
	}
	if v == 0 {
		return inv.Versions[inv.Head]
	}
	return inv.Versions[ocfl.V(v, inv.Head.Padding())]
}

// GetFixity implements ocfl.FixitySource for Inventory
func (inv Inventory) GetFixity(digest string) ocfl.DigestSet {
	paths := inv.Manifest[digest]
	if len(paths) < 1 {
		return nil
	}
	set := ocfl.DigestSet{}
	for fixAlg, fixMap := range inv.Fixity {
		fixMap.EachPath(func(p, fixDigest string) bool {
			if slices.Contains(paths, p) {
				set[fixAlg] = fixDigest
				return false
			}
			return true
		})
	}
	return set
}

func (inv Inventory) Inventory() ocfl.ReadInventory {
	return &readInventory{raw: inv}
}

// Validate checks the inventory's structure and internal consistency. Errors
// and warnings are added to any validations. The returned error wraps all fatal
// errors.
func (inv *Inventory) Validate() *ocfl.Validation {
	v := &ocfl.Validation{}
	if inv.Type.Empty() {
		err := errors.New("missing required field: 'type'")
		v.AddFatal(err)
	}
	ocflV := inv.Type.Spec
	if inv.ID == "" {
		err := errors.New("missing required field: 'id'")
		v.AddFatal(ec(err, codes.E036(ocflV)))
	}
	if inv.Head.IsZero() {
		err := errors.New("missing required field: 'head'")
		v.AddFatal(ec(err, codes.E036(ocflV)))
	}
	if inv.Manifest == nil {
		err := errors.New("missing required field 'manifest'")
		v.AddFatal(ec(err, codes.E041(ocflV)))
	}
	if inv.Versions == nil {
		err := errors.New("missing required field: 'versions'")
		v.AddFatal(ec(err, codes.E041(ocflV)))
	}
	if u, err := url.ParseRequestURI(inv.ID); err != nil || u.Scheme == "" {
		err := fmt.Errorf(`object ID is not a URI: %q`, inv.ID)
		v.AddWarn(ec(err, codes.W005(ocflV)))
	}
	switch inv.DigestAlgorithm {
	case ocfl.SHA512:
		break
	case ocfl.SHA256:
		err := fmt.Errorf(`'digestAlgorithm' is %q`, ocfl.SHA256)
		v.AddWarn(ec(err, codes.W004(ocflV)))
	default:
		err := fmt.Errorf(`'digestAlgorithm' is not %q or %q`, ocfl.SHA512, ocfl.SHA256)
		v.AddFatal(ec(err, codes.E025(ocflV)))
	}
	if err := inv.Head.Valid(); err != nil {
		err = fmt.Errorf("head is invalid: %w", err)
		v.AddFatal(ec(err, codes.E011(ocflV)))
	}
	if strings.Contains(inv.ContentDirectory, "/") {
		err := errors.New("contentDirectory contains '/'")
		v.AddFatal(ec(err, codes.E017(ocflV)))
	}
	if inv.ContentDirectory == "." || inv.ContentDirectory == ".." {
		err := errors.New("contentDirectory is '.' or '..'")
		v.AddFatal(ec(err, codes.E017(ocflV)))
	}
	if inv.Manifest != nil {
		err := inv.Manifest.Valid()
		if err != nil {
			var dcErr *ocfl.MapDigestConflictErr
			var pcErr *ocfl.MapPathConflictErr
			var piErr *ocfl.MapPathInvalidErr
			if errors.As(err, &dcErr) {
				err = ec(err, codes.E096(ocflV))
			} else if errors.As(err, &pcErr) {
				err = ec(err, codes.E101(ocflV))
			} else if errors.As(err, &piErr) {
				err = ec(err, codes.E099(ocflV))
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
				v.AddFatal(ec(err, codes.E107(ocflV)))
			}
		}
	}
	// version names
	var versionNums ocfl.VNums = maps.Keys(inv.Versions)
	if err := versionNums.Valid(); err != nil {
		if errors.Is(err, ocfl.ErrVerEmpty) {
			err = ec(err, codes.E008(ocflV))
		} else if errors.Is(err, ocfl.ErrVNumMissing) {
			err = ec(err, codes.E010(ocflV))
		} else if errors.Is(err, ocfl.ErrVNumPadding) {
			err = ec(err, codes.E012(ocflV))
		}
		v.AddFatal(err)
	}
	if versionNums.Head() != inv.Head {
		err := fmt.Errorf(`version head not most recent version: %s`, inv.Head)
		v.AddFatal(ec(err, codes.E040(ocflV)))
	}
	// version state
	for vname, ver := range inv.Versions {
		if ver == nil {
			err := fmt.Errorf(`missing required version block for %q`, vname)
			v.AddFatal(ec(err, codes.E048(ocflV)))
			continue
		}
		if ver.Created.IsZero() {
			err := fmt.Errorf(`version %s missing required field: 'created'`, vname)
			v.AddFatal(ec(err, codes.E048(ocflV)))
		}
		if ver.Message == "" {
			err := fmt.Errorf("version %s missing recommended field: 'message'", vname)
			v.AddWarn(ec(err, codes.W007(ocflV)))
		}
		if ver.User != nil {
			if ver.User.Name == "" {
				err := fmt.Errorf("version %s user missing required field: 'name'", vname)
				v.AddFatal(ec(err, codes.E054(ocflV)))
			}
			if ver.User.Address == "" {
				err := fmt.Errorf("version %s user missing recommended field: 'address'", vname)
				v.AddWarn(ec(err, codes.W008(ocflV)))
			}
			if u, err := url.ParseRequestURI(ver.User.Address); err != nil || u.Scheme == "" {
				err := fmt.Errorf("version %s user address is not a URI", vname)
				v.AddWarn(ec(err, codes.W009(ocflV)))
			}
		}
		if ver.State == nil {
			err := fmt.Errorf(`version %s missing required field: 'state'`, vname)
			v.AddFatal(ec(err, codes.E048(ocflV)))
			continue
		}
		err := ver.State.Valid()
		if err != nil {
			var dcErr *ocfl.MapDigestConflictErr
			var pcErr *ocfl.MapPathConflictErr
			var piErr *ocfl.MapPathInvalidErr
			if errors.As(err, &dcErr) {
				err = ec(err, codes.E050(ocflV))
			} else if errors.As(err, &pcErr) {
				err = ec(err, codes.E095(ocflV))
			} else if errors.As(err, &piErr) {
				err = ec(err, codes.E052(ocflV))
			}
			v.AddFatal(err)
		}
		// check that each state digest appears in manifest
		for _, digest := range ver.State.Digests() {
			if len(inv.Manifest[digest]) == 0 {
				err := fmt.Errorf("digest in %s state not in manifest: %s", vname, digest)
				v.AddFatal(ec(err, codes.E050(ocflV)))
			}
		}
	}
	//fixity
	for _, fixity := range inv.Fixity {
		err := fixity.Valid()
		if err != nil {
			var dcErr *ocfl.MapDigestConflictErr
			var piErr *ocfl.MapPathInvalidErr
			var pcErr *ocfl.MapPathConflictErr
			if errors.As(err, &dcErr) {
				err = ec(err, codes.E097(ocflV))
			} else if errors.As(err, &piErr) {
				err = ec(err, codes.E099(ocflV))
			} else if errors.As(err, &pcErr) {
				err = ec(err, codes.E101(ocflV))
			}
			v.AddFatal(err)
		}
	}
	return v
}

func (inv *Inventory) setJsonDigest(raw []byte) error {
	digester := ocfl.NewDigester(inv.DigestAlgorithm)
	if digester == nil {
		return fmt.Errorf("%w: %q", ocfl.ErrUnknownAlg, inv.DigestAlgorithm)
	}
	if _, err := io.Copy(digester, bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("digesting inventory: %w", err)
	}
	inv.jsonDigest = digester.String()
	return nil
}

// NewInventory reads the inventory and sets its digest value using the digest
// algorithm. The returned inventory is not fully validated.
func NewInventory(byts []byte) (*Inventory, error) {
	dec := json.NewDecoder(bytes.NewReader(byts))
	dec.DisallowUnknownFields()
	var inv Inventory
	if err := dec.Decode(&inv); err != nil {
		return nil, err
	}
	if err := inv.setJsonDigest(byts); err != nil {
		return nil, err
	}
	return &inv, nil
}

// ValidateInventoryBytes unmarshals the raw json bytes and fully validates the
// internal structure of the inventory. The returned ocfl.Validation will use
// use error codes based of the on the ocfl specificatoin spec.
func ValidateInventoryBytes(raw []byte, spec ocfl.Spec) (inv *Inventory, v *ocfl.Validation) {
	v = &ocfl.Validation{}
	invVals := map[string]any{}
	if err := json.Unmarshal(raw, &invVals); err != nil {
		err = fmt.Errorf("decoding inventory json: %w", err)
		v.AddFatal(ec(err, codes.E033(spec)))
		return nil, v
	}
	const requiredErrMsg = "required field is missing or has unexpected json value"
	const optionalErrMsg = "optional field has unexpected json value"
	id, exists, typeOK := pullJSONValue[string](invVals, `id`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'id'`)
		v.AddFatal(ec(err, codes.E036(spec)))
	}
	typeStr, exists, typeOK := pullJSONValue[string](invVals, `type`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'type'`)
		v.AddFatal(ec(err, codes.E036(spec)))
	}
	if typeStr != "" && typeStr != spec.AsInvType().String() {
		err := fmt.Errorf("invalid inventory type value: %q", typeStr)
		v.AddFatal(ec(err, codes.E038(spec)))
	}
	digestAlg, exists, typeOK := pullJSONValue[string](invVals, `digestAlgorithm`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'digestAlgorithm'`)
		v.AddFatal(ec(err, codes.E036(spec)))
	}
	if digestAlg != "" && digestAlg != ocfl.SHA512 && digestAlg != ocfl.SHA256 {
		err := fmt.Errorf("invalid digest algorithm: %q", digestAlg)
		v.AddFatal(ec(err, codes.E025(spec)))
	}
	head, exists, typeOK := pullJSONValue[string](invVals, `head`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'head'`)
		v.AddFatal(ec(err, codes.E036(spec)))
	}
	manifestVals, exists, typeOK := pullJSONValue[map[string]any](invVals, `manifest`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'manifest'`)
		v.AddFatal(ec(err, codes.E041(spec)))
	}
	versionsVals, exists, typeOK := pullJSONValue[map[string]any](invVals, `versions`)
	if !exists || !typeOK {
		err := errors.New(requiredErrMsg + `: 'versions'`)
		v.AddFatal(ec(err, codes.E043(spec)))
	}
	// FIXME: not sure which error code. E108?
	contentDirectory, exists, typeOK := pullJSONValue[string](invVals, `contentDirectory`)
	if exists && !typeOK {
		// contentDirectory is optional
		err := errors.New(optionalErrMsg + `: 'contentDirectory'`)
		v.AddFatal(err)
	}
	// fixity is optional
	fixityVals, exists, typeOK := pullJSONValue[map[string]any](invVals, `fixity`)
	if exists && !typeOK {
		err := errors.New(optionalErrMsg + `: 'fixity'`)
		v.AddFatal(ec(err, codes.E111(spec)))
	}
	// any remaining values in invVals are invalid
	for extra := range invVals {
		err := fmt.Errorf("inventory json has unexpected field: %q", extra)
		v.AddFatal(err)
	}
	inv = &Inventory{
		ID:               id,
		ContentDirectory: contentDirectory,
		DigestAlgorithm:  digestAlg,
		Fixity:           map[string]ocfl.DigestMap{},
		Versions:         make(map[ocfl.VNum]*Version),
	}
	if err := inv.Type.UnmarshalText([]byte(typeStr)); err != nil {
		v.AddFatal(ec(err, codes.E038(spec)))
	}
	if err := inv.Head.UnmarshalText([]byte(head)); err != nil {
		v.AddFatal(ec(err, codes.E040(spec)))
	}
	var err error
	if inv.Manifest, err = convertJSONDigestMap(manifestVals); err != nil {
		err = fmt.Errorf("invalid manfiest: %w", err)
		v.AddFatal(ec(err, codes.E092(spec)))
	}
	// build versions
	for vnumStr, val := range versionsVals {
		var (
			vnum        ocfl.VNum
			versionVals map[string]any
			userVals    map[string]any
			stateVals   map[string]any
			createdStr  string
			created     time.Time
			message     string
			state       ocfl.DigestMap
			user        *ocfl.User
		)
		if err := ocfl.ParseVNum(vnumStr, &vnum); err != nil {
			err = fmt.Errorf("invalid key %q in versions block: %w", vnumStr, err)
			v.AddFatal(ec(err, codes.E046(spec)))
			continue
		}
		versionErrPrefix := "version '" + vnumStr + "'"
		versionVals, typeOK = val.(map[string]any)
		if !typeOK {
			err := errors.New(versionErrPrefix + ": value is not a json object")
			v.AddFatal(ec(err, codes.E045(spec)))
		}
		createdStr, exists, typeOK = pullJSONValue[string](versionVals, `created`)
		if !exists || !typeOK {
			err := fmt.Errorf("%s: %s: %s", versionErrPrefix, requiredErrMsg, `'created'`)
			v.AddFatal(ec(err, codes.E048(spec)))
		}
		if createdStr != "" {
			if err := created.UnmarshalText([]byte(createdStr)); err != nil {
				err = fmt.Errorf("%s: created: %w", versionErrPrefix, err)
				v.AddFatal(ec(err, codes.E049(spec)))
			}
		}
		stateVals, exists, typeOK = pullJSONValue[map[string]any](versionVals, `state`)
		if !exists || !typeOK {
			err := fmt.Errorf("%s: %s: %q", versionErrPrefix, requiredErrMsg, `state`)
			v.AddFatal(ec(err, codes.E048(spec)))
		}
		// message is optional
		message, exists, typeOK = pullJSONValue[string](versionVals, `message`)
		if exists && !typeOK {
			err := fmt.Errorf("%s: %s: %q", versionErrPrefix, optionalErrMsg, `message`)
			v.AddFatal(ec(err, codes.E094(spec)))
		}
		// user is optional
		userVals, exists, typeOK := pullJSONValue[map[string]any](versionVals, `user`)
		switch {
		case exists && !typeOK:
			err := fmt.Errorf("%s: %s: %q", versionErrPrefix, optionalErrMsg, `user`)
			v.AddFatal(ec(err, codes.E054(spec)))
		case exists:
			var userName, userAddress string
			userName, exists, typeOK = pullJSONValue[string](userVals, `name`)
			if !exists || !typeOK {
				err := fmt.Errorf("%s: user: %s: %q", versionErrPrefix, requiredErrMsg, `name`)
				v.AddFatal(ec(err, codes.E054(spec)))
			}
			// address is optional
			userAddress, exists, typeOK = pullJSONValue[string](userVals, `address`)
			if exists && !typeOK {
				err := fmt.Errorf("%s: user: %s: %q", versionErrPrefix, optionalErrMsg, `address`)
				v.AddFatal(ec(err, codes.E054(spec)))
			}
			user = &ocfl.User{Name: userName, Address: userAddress}
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
		inv.Versions[vnum] = &Version{
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
			v.AddFatal(ec(err, codes.E057(spec)))
			continue
		}
		digests, err := convertJSONDigestMap(digestVals)
		if err != nil {
			err = fmt.Errorf("%s: %w", fixityErrPrefix, err)
			v.AddFatal(ec(err, codes.E057(spec)))
			continue
		}
		inv.Fixity[algStr] = digests
	}
	if err := inv.setJsonDigest(raw); err != nil {
		v.AddFatal(err)
	}
	if v.Err() != nil {
		return nil, v
	}
	v = inv.Validate()
	if v.Err() != nil {
		return nil, v
	}
	return inv, v
}

// writeInventory marshals the value pointed to by inv, writing the json to dir/inventory.json in
// fsys. The digest is calculated using alg and the inventory sidecar is also written to
// dir/inventory.alg
func writeInventory(ctx context.Context, fsys ocfl.WriteFS, inv *Inventory, dirs ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	byts, err := json.Marshal(inv)
	if err != nil {
		return fmt.Errorf("encoding inventory: %w", err)
	}
	// write inventory.json and sidecar
	for _, dir := range dirs {
		invFile := path.Join(dir, inventoryFile)
		sideFile := invFile + "." + inv.DigestAlgorithm
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
func buildInventory(prev ocfl.ReadInventory, commit *ocfl.Commit) (*Inventory, error) {
	if commit.Stage == nil {
		return nil, errors.New("commit is missing new version state")
	}
	if commit.Stage.DigestAlgorithm == "" {
		return nil, errors.New("commit has no digest algorithm")

	}
	if commit.Stage.State == nil {
		commit.Stage.State = ocfl.DigestMap{}
	}
	newInv := &Inventory{
		ID:               commit.ID,
		DigestAlgorithm:  commit.Stage.DigestAlgorithm,
		ContentDirectory: contentDir,
	}
	switch {
	case prev != nil:
		prevInv, ok := prev.(*readInventory)
		if !ok {
			err := errors.New("inventory is not an OCFLv1 inventory")
			return nil, err
		}
		if newInv.DigestAlgorithm != prev.DigestAlgorithm() {
			return nil, fmt.Errorf("commit must use same digest algorithm as existing inventory (%s)", prev.DigestAlgorithm())
		}
		newInv.ID = prev.ID()
		newInv.ContentDirectory = prevInv.raw.ContentDirectory
		newInv.Type = prevInv.raw.Type
		var err error
		newInv.Head, err = prev.Head().Next()
		if err != nil {
			return nil, fmt.Errorf("existing inventory's version scheme doesn't support additional versions: %w", err)
		}
		if !commit.Spec.Empty() {
			// new inventory spec must be >= prev
			if commit.Spec.Cmp(prev.Spec()) < 0 {
				err = fmt.Errorf("new inventory's OCFL spec can't be lower than the existing inventory's (%s)", prev.Spec())
				return nil, err
			}
			newInv.Type = commit.Spec.AsInvType()
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
		newInv.Manifest, err = prev.Manifest().Normalize()
		if err != nil {
			return nil, fmt.Errorf("in existing inventory manifest: %w", err)
		}
		versions := prev.Head().Lineage()
		newInv.Versions = make(map[ocfl.VNum]*Version, len(versions))
		for _, vnum := range versions {
			prevVer := prev.Version(vnum.Num())
			newVer := &Version{
				Created: prevVer.Created(),
				Message: prevVer.Message(),
			}
			newVer.State, err = prevVer.State().Normalize()
			if err != nil {
				return nil, fmt.Errorf("in existing inventory %s state: %w", vnum, err)
			}
			if prevVer.User() != nil {
				newVer.User = &ocfl.User{
					Name:    prevVer.User().Name,
					Address: prevVer.User().Address,
				}
			}
			newInv.Versions[vnum] = newVer
		}
		// transfer fixity
		newInv.Fixity = make(map[string]ocfl.DigestMap, len(prevInv.raw.Fixity))
		for alg, m := range prevInv.raw.Fixity {
			newInv.Fixity[alg], err = m.Normalize()
			if err != nil {
				return nil, fmt.Errorf("in existing inventory %s fixity: %w", alg, err)
			}
		}
	default:
		// FIXME: how whould padding be set for new inventories?
		newInv.Head = ocfl.V(1, 0)
		newInv.Manifest = ocfl.DigestMap{}
		newInv.Fixity = map[string]ocfl.DigestMap{}
		newInv.Versions = map[ocfl.VNum]*Version{}
		newInv.Type = commit.Spec.AsInvType()
	}

	// add new version
	newVersion := &Version{
		State:   commit.Stage.State,
		Created: commit.Created,
		Message: commit.Message,
		User:    &commit.User,
	}
	if newVersion.Created.IsZero() {
		newVersion.Created = time.Now()
	}
	newVersion.Created = newVersion.Created.Truncate(time.Second)
	newInv.Versions[newInv.Head] = newVersion

	// build new manifest and fixity entries
	newContentFunc := func(paths []string) []string {
		// apply user-specified path transform first
		if commit.ContentPathFunc != nil {
			paths = commit.ContentPathFunc(paths)
		}
		contDir := newInv.ContentDirectory
		if contDir == "" {
			contDir = contentDir
		}
		for i, p := range paths {
			paths[i] = path.Join(newInv.Head.String(), contDir, p)
		}
		return paths
	}
	for digest, logicPaths := range newVersion.State {
		if len(newInv.Manifest[digest]) > 0 {
			// version content already exists in the manifest
			continue
		}
		newInv.Manifest[digest] = newContentFunc(slices.Clone(logicPaths))
	}
	if commit.Stage.FixitySource != nil {
		for digest, contentPaths := range newInv.Manifest {
			fixSet := commit.Stage.FixitySource.GetFixity(digest)
			if len(fixSet) < 1 {
				continue
			}
			for fixAlg, fixDigest := range fixSet {
				if newInv.Fixity[fixAlg] == nil {
					newInv.Fixity[fixAlg] = ocfl.DigestMap{}
				}
				for _, cp := range contentPaths {
					fixPaths := newInv.Fixity[fixAlg][fixDigest]
					if !slices.Contains(fixPaths, cp) {
						newInv.Fixity[fixAlg][fixDigest] = append(fixPaths, cp)
					}
				}
			}
		}
	}
	// check that resulting inventory is valid
	if err := newInv.Validate().Err(); err != nil {
		return nil, fmt.Errorf("generated inventory is not valid: %w", err)
	}
	return newInv, nil
}

// readInventory implements ocfl.Inventory
type readInventory struct {
	raw Inventory
}

func (inv *readInventory) MarshalJSON() ([]byte, error) { return json.Marshal(inv.raw) }

func (inv *readInventory) GetFixity(digest string) ocfl.DigestSet { return inv.raw.GetFixity(digest) }

func (inv *readInventory) ContentDirectory() string {
	if c := inv.raw.ContentDirectory; c != "" {
		return c
	}
	return contentDir
}

func (inv *readInventory) Digest() string { return inv.raw.jsonDigest }

func (inv *readInventory) DigestAlgorithm() string { return inv.raw.DigestAlgorithm }

func (inv *readInventory) Head() ocfl.VNum { return inv.raw.Head }

func (inv *readInventory) ID() string { return inv.raw.ID }

func (inv *readInventory) Manifest() ocfl.DigestMap { return inv.raw.Manifest }

func (inv *readInventory) Spec() ocfl.Spec { return inv.raw.Type.Spec }

func (inv *readInventory) Validate() *ocfl.Validation {
	if inv.raw.jsonDigest == "" {
		err := errors.New("inventory was not initialized correctly: missing file digest value")
		v := &ocfl.Validation{}
		v.AddFatal(err)
		return v
	}
	return inv.raw.Validate()
}

func (inv *readInventory) Version(i int) ocfl.ObjectVersion {
	v := inv.raw.Version(i)
	if v == nil {
		return nil
	}
	return &inventoryVersion{ver: v}
}

type inventoryVersion struct {
	ver *Version
}

func (v *inventoryVersion) State() ocfl.DigestMap { return v.ver.State }
func (v *inventoryVersion) Message() string       { return v.ver.Message }
func (v *inventoryVersion) Created() time.Time    { return v.ver.Created }
func (v *inventoryVersion) User() *ocfl.User      { return v.ver.User }

type logicalState struct {
	manifest ocfl.DigestMap
	state    ocfl.DigestMap
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

func pullJSONValue[T any](m map[string]any, key string) (val T, exists bool, typeOK bool) {
	var anyVal any
	anyVal, exists = m[key]
	val, typeOK = anyVal.(T)
	delete(m, key)
	return
}

func convertJSONDigestMap(jsonMap map[string]any) (ocfl.DigestMap, error) {
	m := ocfl.DigestMap{}
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
