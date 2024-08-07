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
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1/codes"
	"golang.org/x/exp/maps"
)

var (
	invSidecarContentsRexp = regexp.MustCompile(`^([a-fA-F0-9]+)\s+inventory\.json[\n]?$`)
	ErrVersionNotFound     = errors.New("version not found in inventory")
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

	// jsonDigest of raw inventory using DigestAlgorithm, set during json marshal/unmarshal
	jsonDigest string
}

// Version represents object version state and metadata
type Version struct {
	Created time.Time      `json:"created"`
	State   ocfl.DigestMap `json:"state"`
	Message string         `json:"message,omitempty"`
	User    *ocfl.User     `json:"user,omitempty"`
}

// UnmarshalJSON decodes the inventory and sets inv's
// digest value for the bytes b.
func (inv *Inventory) UnmarshalJSON(b []byte) error {
	type invAlias Inventory
	var alias invAlias
	if err := json.Unmarshal(b, &alias); err != nil {
		return err
	}
	// digest json bytes
	if d := ocfl.NewDigester(alias.DigestAlgorithm); d != nil {
		if _, err := io.Copy(d, bytes.NewReader(b)); err != nil {
			return err
		}
		alias.jsonDigest = d.String()
	}
	*inv = Inventory(alias)
	if inv.ContentDirectory == "" {
		inv.ContentDirectory = contentDir
	}
	return nil
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
// Inventory wasn't decoded using ValidateInventory or ValidateInventoryReader,
// an empty string is returned.
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

// Validate validates the inventory. It only checks the inventory's structure
// and internal consistency. The inventory's digests are added to validation
func (inv *Inventory) Validate(vld *ocfl.Validation) error {
	var fatal, warn []error
	if inv.Type.Empty() {
		err := errors.New("missing required field: 'type'")
		fatal = append(fatal, err)
	}
	ocflV := inv.Type.Spec
	if inv.ID == "" {
		err := errors.New("missing required field: 'id'")
		fatal = append(fatal, ec(err, codes.E036(ocflV)))
	}
	if inv.Head.IsZero() {
		err := errors.New("missing required field: 'head'")
		fatal = append(fatal, ec(err, codes.E036(ocflV)))
	}
	if inv.Manifest == nil {
		err := errors.New("missing required field 'manifest'")
		fatal = append(fatal, ec(err, codes.E041(ocflV)))
	}
	if inv.Versions == nil {
		err := errors.New("missing required field 'versions'")
		fatal = append(fatal, ec(err, codes.E041(ocflV)))
	}
	if u, err := url.ParseRequestURI(inv.ID); err != nil || u.Scheme == "" {
		err := fmt.Errorf(`object ID is not a URI: %s`, inv.ID)
		warn = append(warn, ec(err, codes.W005(ocflV)))
	}
	switch inv.DigestAlgorithm {
	case ocfl.SHA512:
		break
	case ocfl.SHA256:
		err := fmt.Errorf(`'digestAlgorithm' is %q`, ocfl.SHA256)
		warn = append(warn, ec(err, codes.W004(ocflV)))
	default:
		err := fmt.Errorf(`'digestAlgorithm' is not %q or %q`, ocfl.SHA512, ocfl.SHA256)
		fatal = append(fatal, ec(err, codes.E025(ocflV)))
	}
	if err := inv.Head.Valid(); err != nil {
		err = fmt.Errorf("head is invalid: %w", err)
		fatal = append(fatal, ec(err, codes.E011(ocflV)))
	}
	if strings.Contains(inv.ContentDirectory, "/") {
		err := errors.New("contentDirectory contains '/'")
		fatal = append(fatal, ec(err, codes.E017(ocflV)))
	}
	if inv.ContentDirectory == "." || inv.ContentDirectory == ".." {
		err := errors.New("contentDirectory is '.' or '..'")
		fatal = append(fatal, ec(err, codes.E017(ocflV)))
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
			fatal = append(fatal, err)
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
				fatal = append(fatal, ec(err, codes.E107(ocflV)))
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
		fatal = append(fatal, err)
	}
	if versionNums.Head() != inv.Head {
		err := fmt.Errorf(`version head not most recent version: %s`, inv.Head)
		fatal = append(fatal, ec(err, codes.E040(ocflV)))
	}
	// version state
	for vname, ver := range inv.Versions {
		if ver == nil {
			err := fmt.Errorf(`missing required version block for %q`, vname)
			fatal = append(fatal, ec(err, codes.E048(ocflV)))
			continue
		}
		if ver.Created.IsZero() {
			err := fmt.Errorf(`version %s missing required field: 'created'`, vname)
			fatal = append(fatal, ec(err, codes.E048(ocflV)))
		}
		if ver.Message == "" {
			err := fmt.Errorf("version %s missing recommended field: 'message'", vname)
			warn = append(warn, ec(err, codes.W007(ocflV)))
		}
		if ver.User != nil {
			if ver.User.Name == "" {
				err := fmt.Errorf("version %s user missing required field: 'name'", vname)
				fatal = append(fatal, ec(err, codes.E054(ocflV)))
			}
			if ver.User.Address == "" {
				err := fmt.Errorf("version %s user missing recommended field: 'address'", vname)
				warn = append(warn, ec(err, codes.W008(ocflV)))
			}
			if u, err := url.ParseRequestURI(ver.User.Address); err != nil || u.Scheme == "" {
				err := fmt.Errorf("version %s user address is not a URI", vname)
				warn = append(warn, ec(err, codes.W009(ocflV)))
			}
		}
		if ver.State == nil {
			err := fmt.Errorf(`version %s missing required field: 'state'`, vname)
			fatal = append(fatal, ec(err, codes.E048(ocflV)))
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
			fatal = append(fatal, err)
		}
		// check that each state digest appears in manifest
		for _, digest := range ver.State.Digests() {
			if len(inv.Manifest[digest]) == 0 {
				err := fmt.Errorf("digest in %s state not in manifest: %s", vname, digest)
				fatal = append(fatal, ec(err, codes.E050(ocflV)))
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
			fatal = append(fatal, err)
		}
	}
	// add the version to the validation
	if vld != nil {
		if err := vld.AddInventory(inv.Inventory()); err != nil {
			err = fmt.Errorf("the inventor's digests conflict with previous values: %w", err)
			fatal = append(fatal, err)
		}
		vld.AddFatal(fatal...)
		vld.AddWarn(warn...)
	}
	if len(fatal) > 0 {
		return multierror.Append(nil, fatal...)
	}
	return nil
}

// ValidateInventory fully validates an inventory at path name in fsys.
func ValidateInventory(ctx context.Context, fsys ocfl.FS, name string, result *ocfl.Validation) (inv *Inventory, err error) {
	f, err := fsys.OpenFile(ctx, name)
	if err != nil {
		result.AddFatal(err)
		return
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			result.AddFatal(closeErr)
			err = errors.Join(err, closeErr)
		}
	}()
	byts, err := io.ReadAll(f)
	if err != nil {
		result.AddFatal(err)
		return
	}
	inv, err = ValidateInventoryBytes(byts, result)
	if err != nil {
		return
	}
	ocflV := inv.Type.Spec
	side := name + "." + inv.DigestAlgorithm
	expSum, err := readInventorySidecar(ctx, fsys, side)
	if err != nil {
		inv = nil
		if errors.Is(err, ErrInvSidecarContents) {
			result.AddFatal(ec(err, codes.E061(ocflV)))
			return
		}
		result.AddFatal(ec(err, codes.E058(ocflV)))
		return
	}
	if !strings.EqualFold(inv.jsonDigest, expSum) {
		inv = nil
		shortSum := inv.jsonDigest[:6]
		shortExp := expSum[:6]
		err = fmt.Errorf("inventory's checksum (%s) doen't match expected value in sidecar (%s): %s", shortSum, shortExp, name)
		result.AddFatal(ec(err, codes.E060(ocflV)))
	}
	return
}

func ValidateInventoryBytes(raw []byte, vld *ocfl.Validation) (*Inventory, error) {
	inv, err := NewInventory(raw)
	if err != nil {
		vld.AddFatal(err)
		return nil, err
	}
	if err := inv.Validate(vld); err != nil {
		return nil, err
	}
	return inv, nil
}

// NewInventory reads the inventory and sets its digest value using
// the digest algorithm
func NewInventory(byt []byte) (*Inventory, error) {
	dec := json.NewDecoder(bytes.NewReader(byt))
	dec.DisallowUnknownFields()
	var inv Inventory
	if err := dec.Decode(&inv); err != nil {
		return nil, err
	}
	digester := ocfl.NewDigester(inv.DigestAlgorithm)
	if digester == nil {
		return nil, fmt.Errorf("%w: %q", ocfl.ErrUnknownAlg, inv.DigestAlgorithm)
	}
	if _, err := io.Copy(digester, bytes.NewReader(byt)); err != nil {
		return nil, err
	}
	inv.jsonDigest = digester.String()
	return &inv, nil
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

// readInventorySidecar parses the contents of file as an inventory sidecar, returning
// the stored digest on succecss. An error is returned if the sidecar is not in the expected
// format
func readInventorySidecar(ctx context.Context, fsys ocfl.FS, name string) (digest string, err error) {
	file, err := fsys.OpenFile(ctx, name)
	if err != nil {
		return
	}
	defer file.Close()
	cont, err := io.ReadAll(file)
	if err != nil {
		return
	}
	matches := invSidecarContentsRexp.FindSubmatch(cont)
	if len(matches) != 2 {
		err = fmt.Errorf("reading %s: %w", name, ErrInvSidecarContents)
		return
	}
	digest = string(matches[1])
	return
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
			err := errors.New("previous inventory is not an OCFLv1 inventory")
			return nil, err
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
	if err := newInv.Validate(nil); err != nil {
		return nil, fmt.Errorf("generated inventory is not valid: %w", err)
	}
	return newInv, nil
}

// readInventory implements ocfl.Inventory
type readInventory struct {
	raw Inventory
}

func (inv *readInventory) UnmarshalJSON(b []byte) error { return json.Unmarshal(b, &inv.raw) }

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
