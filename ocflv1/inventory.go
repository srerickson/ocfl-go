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

	digest string // inventory digest using alg
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

// Inventory digest from inventory read
func (inv Inventory) Digest() string {
	return inv.digest
}

// ContentPath returns the content path for the logical path present in the
// state for version vnum. The content path is relative to the object's root
// directory (i.e, as it appears in the inventory manifest). If vnum is empty,
// the inventories head version is used.
func (inv *Inventory) ContentPath(vnum ocfl.VNum, logical string) (string, error) {
	if vnum.IsZero() {
		vnum = inv.Head
	}
	ver, exists := inv.Versions[vnum]
	if !exists {
		return "", fmt.Errorf("no version: %s", vnum)
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

// Copy creates an identical Inventory without any references to values in the
// original inventory
func (inv Inventory) Copy() *Inventory {
	newInv := inv
	newInv.digest = "" // don't copy digest value (read from sidecar)
	newInv.Manifest = newInv.Manifest.Copy()
	newInv.Versions = make(map[ocfl.VNum]*Version, len(inv.Versions))
	for v, ver := range inv.Versions {
		newInv.Versions[v] = &Version{
			Created: ver.Created,
			Message: ver.Message,
			State:   ver.State.Copy(),
		}
		if ver.User != nil {
			newInv.Versions[v].User = &User{
				Name:    ver.User.Name,
				Address: ver.User.Address,
			}
		}
	}
	newInv.Fixity = make(map[string]*digest.Map, len(inv.Fixity))
	for alg, m := range inv.Fixity {
		newInv.Fixity[alg] = m.Copy()
	}
	return &newInv
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

// Index returns an *ocfl.Index that can be modified and used to commit new
// versions of an object. The index returned by Index() is not backed by an FS
// and it does not include manifest and fixity entries. Use IndexFull if
// necessary.
func (inv *Inventory) Index(ver ocfl.VNum) (*ocfl.Index, error) {
	if ver.IsZero() {
		ver = inv.Head
	}
	v, ok := inv.Versions[ver]
	if !ok {
		return nil, errors.New("no such version")
	}
	root := pathtree.NewDir[ocfl.IndexItem]()
	alg := inv.DigestAlgorithm
	eachFunc := func(name, sum string) error {
		manifestPaths := inv.Manifest.DigestPaths(sum)
		info := ocfl.IndexItem{
			Digests:  digest.Set{alg: sum},
			SrcPaths: manifestPaths,
		}
		if len(manifestPaths) > 0 {
			for alg, fix := range inv.Fixity {
				if sum := fix.GetDigest(manifestPaths[0]); sum != "" {
					info.Digests[alg] = sum
				}
			}
		}
		if err := root.SetFile(name, info); err != nil {
			return err
		}
		return nil
	}
	if err := v.State.EachPath(eachFunc); err != nil {
		return nil, err
	}
	idx := &ocfl.Index{}
	idx.SetRoot(root)
	return idx, nil
}

// NewVersionInventory creates a new inventory with a version based on the contents of stage. If prevInv
// is nil, the new inventory includes its versions.
func NewVersionInventory(stage *ocfl.Stage, prevInv *Inventory, created time.Time, msg string, user *User) (*Inventory, error) {
	var inv *Inventory
	var prevMans map[string]*digest.Map // previous manifest + fixity
	if prevInv != nil {
		inv = prevInv.Copy()
		next, err := prevInv.Head.Next()
		if err != nil {
			return nil, err
		}
		inv.Head = next
		prevMans = make(map[string]*digest.Map, 1+len(prevInv.Fixity))
		prevMans[prevInv.DigestAlgorithm] = prevInv.Manifest
		for alg, fix := range prevInv.Fixity {
			prevMans[alg] = fix
		}
	} else {
		inv = &Inventory{
			ID:               "",
			Head:             ocfl.V(1),
			Type:             ocflv1_1.AsInvType(),
			DigestAlgorithm:  stage.DigestAlg().ID(),
			ContentDirectory: contentDir,
			Versions:         map[ocfl.VNum]*Version{},
		}
	}
	// add the new version directory to the Inventory object
	inv.Versions[inv.Head] = &Version{
		Created: created.UTC().Truncate(time.Second),
		User:    user,
		Message: msg,
		State:   stage.VersionState(),
	}
	// func to rename stage source path to manifest path.
	// Stage source paths are relative to the stage root;
	// need to prefix version directory and content.
	rename := func(src string) string {
		return path.Join(inv.Head.String(), inv.ContentDirectory, src)
	}
	newManifests, err := nextManifests(stage, prevMans, rename)
	if err != nil {
		return nil, err
	}
	inv.Manifest = newManifests[inv.DigestAlgorithm]
	delete(newManifests, inv.DigestAlgorithm)
	inv.Fixity = newManifests
	// if err := inv.Validate().Err(); err != nil {
	// 	return nil, fmt.Errorf("new inventory has unexpected errors: %w", err)
	// }
	return inv, nil
}

func nextManifests(stage *ocfl.Stage, prev map[string]*digest.Map, renameFunc func(string) string) (map[string]*digest.Map, error) {
	makers := map[string]*digest.MapMaker{}
	for alg := range prev {
		var err error
		makers[alg], err = digest.MapMakerFrom(prev[alg])
		if err != nil {
			return nil, fmt.Errorf("in previous manifest: %w", err)
		}
	}
	stagemans, err := stage.AllManifests(renameFunc)
	if err != nil {
		return nil, fmt.Errorf("building manifest from stage: %w", err)
	}
	for alg, man := range stagemans {
		if makers[alg] == nil {
			makers[alg] = &digest.MapMaker{}
		}
		if err := man.EachPath(func(name, dig string) error {
			return makers[alg].Add(dig, name)
		}); err != nil {
			return nil, fmt.Errorf("merging previous manifest with stage manifest: %w", err)
		}
	}
	maps := make(map[string]*digest.Map, len(makers))
	for alg, maker := range makers {
		maps[alg] = maker.Map()
	}
	return maps, nil
}
