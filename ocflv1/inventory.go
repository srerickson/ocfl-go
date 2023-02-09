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
	"github.com/srerickson/ocfl/internal/pathtree"
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
type User struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}

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

// Index returns an *ocfl.Index representing the state for a version
// in the inventory.
func (inv Inventory) Index(v int) (*ocfl.Index, error) {
	root := pathtree.NewDir[ocfl.IndexItem]()
	alg := inv.DigestAlgorithm
	eachFunc := func(name, sum string, srcs []string) error {
		info := ocfl.IndexItem{
			Digests:  digest.Set{alg: sum},
			SrcPaths: srcs,
		}
		for alg, fix := range inv.Fixity {
			if sum := fix.GetDigest(srcs[0]); sum != "" {
				info.Digests[alg] = sum
			}
		}
		return root.SetFile(name, info)
	}
	if err := inv.EachStatePath(v, eachFunc); err != nil {
		return nil, err
	}
	idx := &ocfl.Index{}
	idx.SetRoot(root)
	return idx, nil
}

// NextVersionInventory returns a new inventory that is a valid successor for
// the calling inventory. The new inventory will have an incremented version
// number and a new version state based on the contents of the stage. Additional
// arguments are used to set the new version's created timestamp, message
// string, and user information. The new inventory will use lowercase digest
// strings for manifest, version state, and fixity entries. Manifest paths for
// new content files are formed by joining the new version's content directory
// to the content path found in the stage. All staged files with digests not
// found in the calling inventory's manifest must include content paths;
// otherwise, the stage is considered incomplete and an error is returned. If
// the stage uses a digest algorithm that is different from the calling
// inventory's, an error is returned.
func (inv Inventory) NextVersionInventory(stage *ocfl.Stage, created time.Time, msg string, user *User) (*Inventory, error) {
	next, err := inv.Head.Next()
	if err != nil {
		return nil, fmt.Errorf("the inventory's version numbering scheme does not support versions beyond %s: %w", inv.Head, err)
	}
	algid := stage.DigestAlg().ID()
	if inv.DigestAlgorithm != algid {
		return nil, fmt.Errorf("stage and inventory use different digest algorithms: '%s' != '%s'", algid, inv.DigestAlgorithm)
	}
	newInv, err := inv.normalizedCopy()
	if err != nil {
		return nil, fmt.Errorf("in source inventory: %w", err)
	}
	newInv.Head = next
	newInv.Versions[newInv.Head] = &Version{
		Created: created.Truncate(time.Second),
		User:    user,
		Message: msg,
		State:   stage.VersionState(),
	}
	prevManFixity := make(map[string]*digest.Map, 1+len(inv.Fixity))
	prevManFixity[algid] = inv.Manifest
	for alg, fix := range inv.Fixity {
		prevManFixity[alg] = fix
	}
	// func to rename stage source path to manifest path.
	// Stage source paths are relative to the stage root;
	// need to prefix version directory and content.
	rename := func(src string) string {
		return path.Join(newInv.Head.String(), newInv.ContentDirectory, src)
	}
	newManifests, err := mergeStageManifests(stage, prevManFixity, rename)
	if err != nil {
		return nil, err
	}
	newInv.Manifest = newManifests[newInv.DigestAlgorithm]
	delete(newManifests, newInv.DigestAlgorithm)
	newInv.Fixity = newManifests
	return newInv, nil
}

// returns a copy of the inventory with normalized paths
func (inv Inventory) normalizedCopy() (*Inventory, error) {
	man, err := inv.Manifest.Normalized()
	if err != nil {
		return nil, fmt.Errorf("in manifest: %w", err)
	}
	newInv := inv
	newInv.digest = "" // don't copy digest value (read from sidecar)
	newInv.Manifest = man
	newInv.Versions = make(map[ocfl.VNum]*Version, len(inv.Versions))
	for v, ver := range inv.Versions {
		newInv.Versions[v] = &Version{
			Created: ver.Created,
			Message: ver.Message,
		}
		state, err := ver.State.Normalized()
		if err != nil {
			return nil, fmt.Errorf("in version %s state: %w", v, err)
		}
		newInv.Versions[v].State = state
		if ver.User != nil {
			newInv.Versions[v].User = &User{
				Name:    ver.User.Name,
				Address: ver.User.Address,
			}
		}
	}
	newInv.Fixity = make(map[string]*digest.Map, len(inv.Fixity))
	for alg, m := range inv.Fixity {
		fix, err := m.Normalized()
		if err != nil {
			return nil, fmt.Errorf("in %s fixity: %w", alg, err)
		}
		newInv.Fixity[alg] = fix
	}
	return &newInv, nil
}

// mergeStageManifests merges the source paths and digests from the stage into
// the set of manifests, applying the rename function to the source path.
func mergeStageManifests(stage *ocfl.Stage, manifests map[string]*digest.Map, renameFunc func(string) string) (map[string]*digest.Map, error) {
	makers := map[string]*digest.MapMaker{}
	for algid := range manifests {
		m, err := digest.MapMakerFrom(manifests[algid])
		if err != nil {
			return nil, fmt.Errorf("in previous manifest: %w", err)
		}
		makers[algid] = m
	}
	stageAlg := stage.DigestAlg().ID()
	err := stage.Walk(func(p string, n *ocfl.Index) error {
		if n.IsDir() {
			return nil
		}
		digs := n.Val().Digests
		srcs := n.Val().SrcPaths
		if digs[stageAlg] == "" {
			return fmt.Errorf("missing '%s' for path '%s'", stageAlg, p)
		}
		for algID := range digs {
			if makers[algID] == nil {
				makers[algID] = &digest.MapMaker{}
			}
			for _, src := range srcs {
				if renameFunc != nil {
					src = renameFunc(src)
				}
				// It's ok if the path has already been added to the map maker
				// since another logical path in the stage might use the same
				// source path. The error we are concerned with is whether the
				// source path was previously added with a different digest.
				err := makers[algID].Add(digs[algID], src)
				if err != nil && !errors.Is(err, digest.ErrMapMakerExists) {
					return err
				}
			}
		}
		// the primary digest for each logical path in the stage should have a
		// content path in the new manifest. check that the maker for the new
		// manifest has an entry for the digest.
		if !makers[stageAlg].HasDigest(digs[stageAlg]) {
			return fmt.Errorf("missing content path for '%s'", p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	maps := make(map[string]*digest.Map, len(makers))
	for alg, maker := range makers {
		maps[alg] = maker.Map()
	}
	return maps, nil
}

// NewInventory creates an initial inventory for the first version of an object
// based on the contets of the stage. The new inventory will use lowercase
// digest strings for manifest, version state, and fixity entries. Manifest
// paths for new content files are formed by joining the new version's content
// directory to the content path found in the stage. All staged files must have
// content paths or else the stage is considered incomplete and an error is
// returne.
func NewInventory(stage *ocfl.Stage, id string, contDir string, padding int, created time.Time, msg string, user *User) (*Inventory, error) {
	if contDir == "" {
		contDir = contentDir
	}
	head := ocfl.V(1, padding)
	if err := head.Valid(); err != nil {
		return nil, fmt.Errorf("invalid padding: %d", padding)
	}
	if alg := stage.DigestAlg().ID(); !algorithms[alg] {
		return nil, fmt.Errorf("invalid digest algorithm %s", alg)
	}
	inv := &Inventory{
		ID:               id,
		Head:             head,
		Type:             defaultSpec.AsInvType(),
		DigestAlgorithm:  stage.DigestAlg().ID(),
		ContentDirectory: contDir,
		Versions: map[ocfl.VNum]*Version{
			head: {
				Created: created.Truncate(time.Second),
				User:    user,
				Message: msg,
				State:   stage.VersionState(),
			},
		},
	}
	makers := map[string]*digest.MapMaker{}
	walkFn := func(p string, n *ocfl.Index) error {
		if n.IsDir() {
			return nil
		}
		digs := n.Val().Digests
		srcs := n.Val().SrcPaths
		if digs[inv.DigestAlgorithm] == "" {
			return fmt.Errorf("missing %s for '%s'", inv.DigestAlgorithm, p)
		}
		// for a new inventory every file in the stage must have a source
		if len(srcs) == 0 {
			return fmt.Errorf("missing content path for '%s'", p)
		}
		for algID := range digs {
			if makers[algID] == nil {
				makers[algID] = &digest.MapMaker{}
			}
			for _, src := range srcs {
				src := path.Join(inv.Head.String(), inv.ContentDirectory, src)
				// Add content paths to manifest and fixity. It's ok if the same
				// path is added multiple times, but only if the digest is the
				// same.
				err := makers[algID].Add(digs[algID], src)
				if err != nil && !errors.Is(err, digest.ErrMapMakerExists) {
					return err
				}
			}
		}
		return nil
	}
	if err := stage.Walk(walkFn); err != nil {
		return nil, fmt.Errorf("in new inventory stage: %w", err)
	}
	maps := map[string]*digest.Map{}
	for alg, maker := range makers {
		maps[alg] = maker.Map()
	}
	inv.Manifest = maps[inv.DigestAlgorithm]
	if inv.Manifest == nil {
		inv.Manifest = &digest.Map{}
	}
	delete(maps, inv.DigestAlgorithm)
	inv.Fixity = maps
	return inv, nil
}
