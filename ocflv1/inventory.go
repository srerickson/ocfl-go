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
	ID               string                 `json:"id"`
	Type             ocfl.InvType           `json:"type"`
	DigestAlgorithm  string                 `json:"digestAlgorithm"`
	Head             ocfl.VNum              `json:"head"`
	ContentDirectory string                 `json:"contentDirectory,omitempty"`
	Manifest         *digest.Map            `json:"manifest"`
	Versions         map[ocfl.VNum]*Version `json:"versions"`
	Fixity           map[string]*digest.Map `json:"fixity,omitempty"`

	digest string     // inventory digest using alg
	alg    digest.Alg // resolved digest algorithm
}

// Version represents object version state and metadata
type Version struct {
	Created time.Time   `json:"created"`
	State   *digest.Map `json:"state"`
	Message string      `json:"message,omitempty"`
	User    *User       `json:"user,omitempty"`
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
	if ver == nil || ver.State == nil {
		return fmt.Errorf("%w: with index %d", ErrVersionNotFound, v)
	}
	if inv.Manifest == nil {
		return errors.New("inventory has no manifest")
	}
	return ver.State.EachPath(func(lpath string, digest string) error {
		if digest == "" {
			return fmt.Errorf("missing digest for %s", lpath)
		}
		srcs := inv.Manifest.DigestPaths(digest)
		if len(srcs) == 0 {
			return fmt.Errorf("missing manifest entry for %s", digest)
		}
		return fn(lpath, digest, srcs)
	})
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
		return "", fmt.Errorf("%w: %s", ErrInvSidecarOpen, err.Error())
	}
	matches := invSidecarContentsRexp.FindSubmatch(cont)
	if len(matches) != 2 {
		return "", fmt.Errorf("%w: %s", ErrInvSidecarContents, string(cont))
	}
	return string(matches[1]), nil
}

func (inv *Inventory) normalizeDigests() error {
	// manifest + all version state + all fixity
	invMaps := make([]*digest.Map, 0, 1+len(inv.Versions)+len(inv.Fixity))
	if inv.Manifest != nil {
		invMaps = append(invMaps, inv.Manifest)
	}
	for _, v := range inv.Versions {
		if v.State == nil {
			continue
		}
		invMaps = append(invMaps, v.State)
	}
	for _, f := range inv.Fixity {
		if f == nil {
			continue
		}
		invMaps = append(invMaps, f)
	}
	for _, m := range invMaps {
		if m.HasUppercaseDigests() {
			newMap, err := m.Normalized()
			if err != nil {
				return err
			}
			*m = newMap
		}
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
	newState := stage.State()
	inv.Versions[inv.Head] = &Version{
		State:   newState,
		Message: msg,
		Created: created,
		User:    user,
	}
	if inv.ContentDirectory == "" {
		inv.ContentDirectory = contentDir
	}
	if inv.Manifest == nil {
		inv.Manifest = &digest.Map{}
	}
	// pathTransformation applied to stage export's manifest and fixity
	// to generate inventory's manifest and fixity
	pathTransform := func(digest string, paths []string) []string {
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
	// generate new manifest and fixity entries
	manifestMaker, err := digest.MapMakerFrom(*inv.Manifest)
	if err != nil {
		return fmt.Errorf("existing inventory's manifest has errors: %w", err)
	}
	fixityMakers := make(map[string]*digest.MapMaker, len(inv.Fixity))
	for alg, fix := range inv.Fixity {
		fixityMakers[alg], err = digest.MapMakerFrom(*fix)
		if err != nil {
			return fmt.Errorf("existing inventory's fixity has errors: %w", err)
		}
	}
	for _, dig := range newState.AllDigests() {
		// if the digest is present in the previous manifest, ignore it.
		if inv.Manifest.HasDigest(dig) {
			continue
		}
		// New content paths are based on the logical paths found in the stage
		// state.
		contentPaths := pathTransform(dig, newState.DigestPaths(dig))
		if err := manifestMaker.AddPaths(dig, contentPaths...); err != nil {
			return fmt.Errorf("while generating new manifest: %w", err)
		}
		for alg, val := range stage.GetFixity(dig) {
			if fixityMakers[alg] == nil {
				fixityMakers[alg] = &digest.MapMaker{}
			}
			if err := fixityMakers[alg].AddPaths(val, contentPaths...); err != nil {
				return fmt.Errorf("while generating new fixity: %w", err)
			}
		}
	}
	inv.Manifest = manifestMaker.Map()
	inv.Fixity = map[string]*digest.Map{}
	for alg, fixmaker := range fixityMakers {
		inv.Fixity[alg] = fixmaker.Map()
	}
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
		Manifest: *inv.Manifest,
		Map:      *ver.State,
		Message:  ver.Message,
		Created:  ver.Created,
		Alg:      alg,
	}, nil
}
