package ocflv1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/srerickson/ocfl-go"
)

var (
	invSidecarContentsRexp = regexp.MustCompile(`^([a-fA-F0-9]+)\s+inventory\.json[\n]?$`)
	ErrVersionNotFound     = errors.New("version not found in inventory")
)

// RawInventory represents contents of an OCFL v1.x inventory.json file
type RawInventory struct {
	ID               string                    `json:"id"`
	Type             ocfl.InvType              `json:"type"`
	DigestAlgorithm  string                    `json:"digestAlgorithm"`
	Head             ocfl.VNum                 `json:"head"`
	ContentDirectory string                    `json:"contentDirectory,omitempty"`
	Manifest         ocfl.DigestMap            `json:"manifest"`
	Versions         map[ocfl.VNum]*Version    `json:"versions"`
	Fixity           map[string]ocfl.DigestMap `json:"fixity,omitempty"`

	// digest of raw inventory using DigestAlgorithm, set during json marshal/unmarshal
	digest string
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
func (inv *RawInventory) UnmarshalJSON(b []byte) error {
	type invAlias RawInventory
	var alias invAlias
	if err := json.Unmarshal(b, &alias); err != nil {
		return err
	}
	// digest json bytes
	if d := ocfl.NewDigester(alias.DigestAlgorithm); d != nil {
		if _, err := io.Copy(d, bytes.NewReader(b)); err != nil {
			return err
		}
		alias.digest = d.String()
	}
	*inv = RawInventory(alias)
	if inv.ContentDirectory == "" {
		inv.ContentDirectory = contentDir
	}
	return nil
}

func (inv *RawInventory) MarshalJSON() ([]byte, error) {
	type invAlias RawInventory
	alias := (*invAlias)(inv)
	byts, err := json.Marshal(alias)
	if err != nil {
		return nil, err
	}
	if d := ocfl.NewDigester(inv.DigestAlgorithm); d != nil {
		if _, err := io.Copy(d, bytes.NewReader(byts)); err != nil {
			return nil, err
		}
		inv.digest = d.String()
	}
	return byts, nil
}

// VNums returns a sorted slice of VNums corresponding to the keys in the
// inventory's 'versions' block.
func (inv RawInventory) VNums() []ocfl.VNum {
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
func (inv RawInventory) Digest() string {
	return inv.digest
}

// ContentPath resolves the logical path from the version state with number v to
// a content path (i.e., a manifest path). The content path is relative to the
// object's root directory. If v is zero, the inventories head version is used.
func (inv RawInventory) ContentPath(v int, logical string) (string, error) {
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
func (inv RawInventory) Version(v int) *Version {
	if inv.Versions == nil {
		return nil
	}
	if v == 0 {
		return inv.Versions[inv.Head]
	}
	return inv.Versions[ocfl.V(v, inv.Head.Padding())]
}

// GetFixity implements ocfl.FixitySource for Inventory
func (inv RawInventory) GetFixity(digest string) ocfl.DigestSet {
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

// writeInventory marshals the value pointed to by inv, writing the json to dir/inventory.json in
// fsys. The digest is calculated using alg and the inventory sidecar is also written to
// dir/inventory.alg
func writeInventory(ctx context.Context, fsys ocfl.WriteFS, inv *RawInventory, dirs ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	digester := ocfl.NewDigester(inv.DigestAlgorithm)
	if digester == nil {
		return fmt.Errorf("%w: %q", ocfl.ErrUnknownAlg, inv.DigestAlgorithm)
	}
	byts, err := json.Marshal(inv)
	if err != nil {
		return fmt.Errorf("encoding inventory: %w", err)
	}
	_, err = io.Copy(digester, bytes.NewReader(byts))
	if err != nil {
		return err
	}
	invDigest := digester.String()
	// write inventory.json and sidecar
	for _, dir := range dirs {
		invFile := path.Join(dir, inventoryFile)
		sideFile := invFile + "." + inv.DigestAlgorithm
		sideContent := invDigest + " " + inventoryFile + "\n"
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
func buildInventory(prev ocfl.Inventory, commit *ocfl.Commit) (*RawInventory, error) {
	if commit.Stage == nil {
		return nil, errors.New("commit is missing new version state")
	}
	if commit.Stage.DigestAlgorithm == "" {
		return nil, errors.New("commit has no digest algorithm")

	}
	if commit.Stage.State == nil {
		commit.Stage.State = ocfl.DigestMap{}
	}
	newInv := &RawInventory{
		ID:               commit.ID,
		DigestAlgorithm:  commit.Stage.DigestAlgorithm,
		ContentDirectory: contentDir,
	}
	switch {
	case prev != nil:
		prevInv, ok := prev.(*inventory)
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
		versions := prev.Head().AsHead()
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

type logicalState struct {
	manifest ocfl.DigestMap
	state    ocfl.DigestMap
}

func (inv *RawInventory) logicalState(i int) logicalState {
	var state ocfl.DigestMap
	if v := inv.Version(i); v != nil {
		state = v.State
	}
	return logicalState{
		manifest: inv.Manifest,
		state:    state,
	}
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

// inventory implements ocfl.Inventory
type inventory struct {
	raw RawInventory
}

func (inv *inventory) UnmarshalJSON(b []byte) error           { return json.Unmarshal(b, &inv.raw) }
func (inv *inventory) MarshalJSON() ([]byte, error)           { return json.Marshal(inv.raw) }
func (inv *inventory) GetFixity(digest string) ocfl.DigestSet { return inv.raw.GetFixity(digest) }
func (inv *inventory) ContentDirectory() string               { return inv.raw.ContentDirectory }
func (inv *inventory) DigestAlgorithm() string                { return inv.raw.DigestAlgorithm }
func (inv *inventory) Head() ocfl.VNum                        { return inv.raw.Head }
func (inv *inventory) ID() string                             { return inv.raw.ID }
func (inv *inventory) Manifest() ocfl.DigestMap               { return inv.raw.Manifest }
func (inv *inventory) Spec() ocfl.Spec                        { return inv.raw.Type.Spec }
func (inv *inventory) Version(i int) ocfl.ObjectVersion {
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
