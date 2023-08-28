package ocflv1

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
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

	digest string     // inventory digest using alg
	alg    digest.Alg // resolved digest algorithm
}

// Version represents object version state and metadata
type Version struct {
	Created time.Time      `json:"created"`
	State   ocfl.DigestMap `json:"state"`
	Message string         `json:"message,omitempty"`
	User    *User          `json:"user,omitempty"`
}

// User represent a Version's user entry
type User ocfl.User

// VNums returns a sorted slice of VNums corresponding to the keys in the
// inventory's 'versions' block.
func (inv *Inventory) VNums() []ocfl.VNum {
	vnums := make([]ocfl.VNum, len(inv.Versions))
	i := 0
	for v := range inv.Versions {
		vnums[i] = v
		i++
	}
	sort.Sort(ocfl.VNums(vnums))
	return vnums
}

// Alg returns the inventory's digest algorithm as a digest.Alg. It panics if the
// digest algorithm isn't set or is unrecognized.
func (inv Inventory) Alg() digest.Alg {
	if inv.alg == nil {
		alg, err := digest.Get(inv.DigestAlgorithm)
		if err != nil {
			panic(err)
		}
		inv.alg = alg
	}
	return inv.alg
}

// Inventory digest from inventory read
func (inv Inventory) Digest() string {
	return inv.digest
}

// ContentPath resolves the logical path from the version state with number v to
// a content path (i.e., a manifest path). The content path is relative to the
// object's root directory. If v is zero, the inventories head version is used.
func (inv Inventory) ContentPath(v int, logical string) (string, error) {
	ver := inv.GetVersion(v)
	if ver == nil {
		return "", ErrVersionNotFound
	}
	sum := ver.State.GetDigest(logical)
	if sum == "" {
		return "", fmt.Errorf("no path: %s", logical)
	}
	paths := inv.Manifest.DigestPaths(sum)
	if len(paths) == 0 {
		return "", fmt.Errorf("missing manifest entry for: %s", sum)
	}
	return paths[0], nil
}

// GetVersion returns the version entry from the entry with number v. If v is 0,
// the head version is used. If no version entry exists, nil is returned
func (inv Inventory) GetVersion(v int) *Version {
	if inv.Versions == nil {
		return nil
	}
	if v == 0 {
		return inv.Versions[inv.Head]
	}
	return inv.Versions[ocfl.V(v, inv.Head.Padding())]
}

// EachStatePath calls fn for each file in the state for the version with number
// v. If v is 0, the inventory's head version is used. The function fn is called
// with the logical file path from the version state, the file's digest as it
// appears in the version state, and a slice of content paths associated with
// the digest from the inventory manifest. The digest will always be a non-empty
// string and the slice of content paths will always include at least one entry.
// If the digest and content paths for a logical path are not found (i.e., the
// inventory is invalid), fn is not called; instead EachStatePath return an
// error. If any call to fn returns a non-nil error, no additional calls are
// made and the error is returned by EachStatePath. If no version state with
// number v is present in the inventory, an error is returned.
func (inv Inventory) EachStatePath(v int, fn func(f string, digest string, conts []string) error) error {
	ver := inv.GetVersion(v)
	if ver == nil {
		return fmt.Errorf("%w: with index %d", ErrVersionNotFound, v)
	}
	var err error
	ver.State.EachPath(func(lpath string, digest string) bool {
		if digest == "" {
			err = fmt.Errorf("missing digest for %s", lpath)
			return false
		}
		srcs := inv.Manifest.DigestPaths(digest)
		if len(srcs) == 0 {
			err = fmt.Errorf("missing manifest entry for %s", digest)
			return false
		}
		err = fn(lpath, digest, srcs)
		return err == nil
	})
	return err
}

// WriteInventory marshals the value pointed to by inv, writing the json to dir/inventory.json in
// fsys. The digest is calculated using alg and the inventory sidecar is also written to
// dir/inventory.alg
func WriteInventory(ctx context.Context, fsys ocfl.WriteFS, inv *Inventory, dirs ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	alg, err := digest.Get(inv.DigestAlgorithm)
	if err != nil {
		return err
	}
	checksum := alg.New()
	byt, err := json.MarshalIndent(inv, "", " ")
	if err != nil {
		return fmt.Errorf("encoding inventory: %w", err)
	}
	_, err = io.Copy(checksum, bytes.NewBuffer(byt))
	if err != nil {
		return err
	}
	sum := hex.EncodeToString(checksum.Sum(nil))
	// write inventory.json and sidecar
	for _, dir := range dirs {
		invFile := path.Join(dir, inventoryFile)
		sideFile := invFile + "." + inv.DigestAlgorithm
		_, err = fsys.Write(ctx, invFile, bytes.NewBuffer(byt))
		if err != nil {
			return fmt.Errorf("write inventory failed: %w", err)
		}
		_, err = fsys.Write(ctx, sideFile, strings.NewReader(sum+" "+inventoryFile+"\n"))
		if err != nil {
			return fmt.Errorf("write inventory sidecar failed: %w", err)
		}
	}
	return nil
}

// readInventorySidecar parses the contents of file as an inventory sidecar, returning
// the stored digest on succecss. An error is returned if the sidecar is not in the expected
// format
func readInventorySidecar(ctx context.Context, fsys ocfl.FS, name string) (string, error) {
	file, err := fsys.OpenFile(ctx, name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	cont, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	matches := invSidecarContentsRexp.FindSubmatch(cont)
	if len(matches) != 2 {
		return "", fmt.Errorf("%w: %s", ErrInvSidecarContents, string(cont))
	}
	return string(matches[1]), nil
}

func (inv *Inventory) normalizeDigests() error {
	// manifest + all version state + all fixity
	invMaps := make([]*ocfl.DigestMap, 0, 1+len(inv.Versions)+len(inv.Fixity))
	invMaps = append(invMaps, &inv.Manifest)
	for _, v := range inv.Versions {
		invMaps = append(invMaps, &v.State)
	}
	for _, f := range inv.Fixity {
		invMaps = append(invMaps, &f)
	}
	for _, m := range invMaps {
		newMap, err := m.Normalize()
		if err != nil {
			return err
		}
		*m = newMap
	}
	return nil
}

// AddVersion builds a new version and updates the inventory using state and
// manifest from the provided stage. The new version will have have the given
// message, user, and created timestamp. The function pathfn, is used The
// inventories head is incremented. The function pathfn is optional and is is
// used to customize content paths for new manifest entries.
func (inv *Inventory) AddVersion(stage *ocfl.Stage, msg string, user *User, created time.Time, pathfn func(string, []string) []string) (err error) {
	if inv.Head, err = inv.Head.Next(); err != nil {
		return fmt.Errorf("inventory's version scheme doesn't allow additional versions: %w", err)
	}
	// normalize all digests in the inventory. If we don't do this
	// non-normalized digests in previous version states might cause problems
	// since the updated manifest/fixity will be normalized.
	if err = inv.normalizeDigests(); err != nil {
		return fmt.Errorf("while normalizing inventory digests: %w", err)
	}
	if inv.Versions == nil {
		inv.Versions = map[ocfl.VNum]*Version{}
	}
	if inv.DigestAlgorithm == "" {
		inv.DigestAlgorithm = stage.Alg.ID()
		inv.alg = stage.Alg
	}
	if inv.DigestAlgorithm != stage.Alg.ID() {
		return fmt.Errorf("inventory uses %s: can't update with stage using %s", inv.DigestAlgorithm, stage.Alg.ID())
	}
	inv.Versions[inv.Head] = &Version{
		State:   stage.State,
		Message: msg,
		Created: created,
		User:    user,
	}
	if inv.ContentDirectory == "" {
		inv.ContentDirectory = contentDir
	}
	// newContentRemap is applied to the stage state to generate a DigestMap
	// with new manifest/fixity entries
	newContentRemap := func(digest string, paths []string) []string {
		// if digest is in existing manifest, ignore this digest
		if inv.Manifest.HasDigest(digest) {
			return nil
		}
		// apply user-specified path transform first
		if pathfn != nil {
			paths = pathfn(digest, paths)
		}
		// prefix paths with "{vnum}/content"
		for i, p := range paths {
			paths[i] = path.Join(inv.Head.String(), inv.ContentDirectory, p)

		}
		return paths
	}
	// create new manifest entries and merge with existing manifest
	newManifestDigests, err := stage.State.Remap(newContentRemap)
	if err != nil {
		return err
	}
	inv.Manifest, err = inv.Manifest.Merge(newManifestDigests, false)
	if err != nil {
		return err
	}
	// create new fixity entries and merge with existing fixity
	newFixityDigests := map[string]map[string][]string{}
	newManifestDigests.EachDigest(func(digest string, paths []string) bool {
		for fixAlg, fixDigest := range stage.GetFixity(digest) {
			if newFixityDigests[fixAlg] == nil {
				newFixityDigests[fixAlg] = map[string][]string{}
			}
			newFixityDigests[fixAlg][fixDigest] = paths
		}
		return true
	})
	if len(newFixityDigests) > 0 && inv.Fixity == nil {
		inv.Fixity = map[string]ocfl.DigestMap{}
	}
	for fixAlg, fixMap := range newFixityDigests {
		newFixMap, err := ocfl.NewDigestMap(fixMap)
		if err != nil {
			return err
		}
		inv.Fixity[fixAlg], err = inv.Fixity[fixAlg].Merge(newFixMap, false)
		if err != nil {
			return err
		}
	}
	// check that resulting inventory is valid
	if err := inv.Validate().Err(); err != nil {
		return fmt.Errorf("generated inventory is not valid (this is probably a bug): %w", err)
	}
	return nil
}

func (inv Inventory) objectState(v int) (*ocfl.ObjectState, error) {
	ver := inv.GetVersion(v)
	if ver == nil {
		return nil, fmt.Errorf("%w: with index %d", ErrVersionNotFound, v)
	}
	alg, err := digest.Get(inv.DigestAlgorithm)
	if err != nil {
		return nil, err
	}

	return &ocfl.ObjectState{
		DigestMap: ver.State,
		Manifest:  inv.Manifest,
		Message:   ver.Message,
		Created:   ver.Created,
		Alg:       alg,
		VNum:      inv.Head,
		Spec:      inv.Type.Spec,
	}, nil
}
