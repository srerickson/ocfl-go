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

// Inventory represents contents of an OCFL v1.x inventory.json file
type Inventory struct {
	ID               string                    `json:"id"`
	Type             ocfl.InvType              `json:"type"`
	DigestAlgorithm  string                    `json:"digestAlgorithm"`
	Head             ocfl.VNum                 `json:"head"`
	ContentDirectory string                    `json:"contentDirectory,omitempty"`
	Manifest         ocfl.DigestMap            `json:"manifest"`
	Versions         map[ocfl.VNum]*Version    `json:"versions"`
	Fixity           map[string]ocfl.DigestMap `json:"fixity,omitempty"`

	// digest of raw inventory using DigestAlgorithm, set during json unmarshal
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
		alias.digest = d.String()
	}
	*inv = Inventory(alias)
	if inv.ContentDirectory == "" {
		inv.ContentDirectory = contentDir
	}
	return nil
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
	return inv.digest
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

// WriteInventory marshals the value pointed to by inv, writing the json to dir/inventory.json in
// fsys. The digest is calculated using alg and the inventory sidecar is also written to
// dir/inventory.alg
func WriteInventory(ctx context.Context, fsys ocfl.WriteFS, inv *Inventory, dirs ...string) error {
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

// NormalizedCopy returns a copy of the inventory with all digests
// normalized.
func (inv Inventory) NormalizedCopy() (*Inventory, error) {
	newInv := inv
	newInv.digest = "" // don't copy digest value (read from sidecar)
	var err error
	newInv.Manifest, err = inv.Manifest.Normalize()
	if err != nil {
		return nil, fmt.Errorf("in manifest: %w", err)
	}
	newInv.Versions = make(map[ocfl.VNum]*Version, len(inv.Versions))
	for v, ver := range inv.Versions {
		newInv.Versions[v] = &Version{
			Created: ver.Created,
			Message: ver.Message,
		}
		newInv.Versions[v].State, err = ver.State.Normalize()
		if err != nil {
			return nil, fmt.Errorf("in version %s state: %w", v, err)
		}
		if ver.User != nil {
			newInv.Versions[v].User = &ocfl.User{
				Name:    ver.User.Name,
				Address: ver.User.Address,
			}
		}
	}
	newInv.Fixity = make(map[string]ocfl.DigestMap, len(inv.Fixity))
	for alg, m := range inv.Fixity {
		newInv.Fixity[alg], err = m.Normalize()
		if err != nil {
			return nil, fmt.Errorf("in %s fixity: %w", alg, err)
		}
	}
	return &newInv, nil
}

// NextInventory builds a new inventory from base with an incremented HEAD and
// a new version block. The new manifest will include the previous manifest
// plus new entries for new content. Manifest paths are generated by passing
// new content paths through contentPathFn. Fixer is used to provide new fixity
// values.
func NewInventory(base *Inventory, ver *Version, fixer ocfl.FixitySource, contentPathFn ocfl.RemapFunc) (*Inventory, error) {
	if ver == nil || ver.State == nil {
		return nil, errors.New("missing new version state")
	}
	if base == nil {
		base = &Inventory{
			ContentDirectory: contentDir,
			Type:             defaultSpec.AsInvType(),
		}
	}
	newHead, err := base.Head.Next()
	if err != nil {
		return nil, fmt.Errorf("inventory's version scheme doesn't allow additional versions: %w", err)
	}
	nextInv, err := base.NormalizedCopy()
	if err != nil {
		return nil, fmt.Errorf("errors in base inventory: %w", err)
	}
	nextInv.Head = newHead
	// normalize all digests in the inventory. If we don't do this
	// non-normalized digests in previous version states might cause problems
	// since the updated manifest/fixity will be normalized.
	if nextInv.Manifest == nil {
		nextInv.Manifest = ocfl.DigestMap{}
	}
	if nextInv.Fixity == nil {
		nextInv.Fixity = map[string]ocfl.DigestMap{}
	}
	if nextInv.Versions == nil {
		nextInv.Versions = map[ocfl.VNum]*Version{}
	}
	nextInv.Versions[nextInv.Head] = ver

	// build new manifest and fixity entries
	newContentFunc := func(paths []string) []string {
		// apply user-specified path transform first
		if contentPathFn != nil {
			paths = contentPathFn(paths)
		}
		for i, p := range paths {
			paths[i] = path.Join(nextInv.Head.String(), nextInv.ContentDirectory, p)
		}
		return paths
	}
	for digest, logicPaths := range ver.State {
		if len(nextInv.Manifest[digest]) > 0 {
			// exists
			continue
		}
		nextInv.Manifest[digest] = newContentFunc(slices.Clone(logicPaths))
	}
	if fixer != nil {
		for digest, contentPaths := range nextInv.Manifest {
			fixSet := fixer.GetFixity(digest)
			if len(fixSet) < 1 {
				continue
			}
			for fixAlg, fixDigest := range fixSet {
				if nextInv.Fixity[fixAlg] == nil {
					nextInv.Fixity[fixAlg] = ocfl.DigestMap{}
				}
				for _, cp := range contentPaths {
					fixPaths := nextInv.Fixity[fixAlg][fixDigest]
					if !slices.Contains(fixPaths, cp) {
						nextInv.Fixity[fixAlg][fixDigest] = append(fixPaths, cp)
					}
				}
			}
		}
	}
	// check that resulting inventory is valid
	if err := nextInv.Validate().Err(); err != nil {
		return nil, fmt.Errorf("generated inventory is not valid: %w", err)
	}
	return nextInv, nil
}

type logicalState struct {
	manifest ocfl.DigestMap
	state    ocfl.DigestMap
}

func (inv *Inventory) logicalState(i int) logicalState {
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

type inventory struct {
	inv Inventory
}

func (inv *inventory) UnmarshalJSON(b []byte) error           { return json.Unmarshal(b, &inv.inv) }
func (inv *inventory) MarshalJSON() ([]byte, error)           { return json.Marshal(inv.inv) }
func (inv *inventory) GetFixity(digest string) ocfl.DigestSet { return inv.inv.GetFixity(digest) }
func (inv *inventory) DigestAlgorithm() string                { return inv.inv.DigestAlgorithm }
func (inv *inventory) Head() ocfl.VNum                        { return inv.inv.Head }
func (inv *inventory) ID() string                             { return inv.inv.ID }
func (inv *inventory) Manifest() ocfl.DigestMap               { return inv.inv.Manifest }
func (inv *inventory) Spec() ocfl.Spec                        { return inv.inv.Type.Spec }
func (inv *inventory) Version(i int) ocfl.ObjectVersion {
	v := inv.inv.Version(i)
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
