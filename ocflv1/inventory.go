package ocflv1

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend"
	"github.com/srerickson/ocfl/digest"
)

var (
	invSidecarContentsRexp = regexp.MustCompile(`^([a-fA-F0-9]+)\s+inventory\.json[\n]?$`)
)

// Inventory represents contents of an OCFL v1.x inventory.json file
type Inventory struct {
	ID               string                     `json:"id"`
	Type             ocfl.InvType               `json:"type"`
	DigestAlgorithm  digest.Alg                 `json:"digestAlgorithm"`
	Head             ocfl.VNum                  `json:"head"`
	ContentDirectory string                     `json:"contentDirectory,omitempty"`
	Manifest         *digest.Map                `json:"manifest"`
	Versions         map[ocfl.VNum]*Version     `json:"versions"`
	Fixity           map[digest.Alg]*digest.Map `json:"fixity,omitempty"`

	digest string
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
	Name    string `json:"name,omitempty"`
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
	sort.Sort(ocfl.VNumSeq(vnums))
	return vnums
}

// VState returns a pointer to a VState representing the logical state of the
// version num. If num is an empty value, the head version is used. If the
// version does not exist for num, nil is returned.
func (inv *Inventory) VState(num ocfl.VNum) *ocfl.VState {
	if num == ocfl.V0 {
		num = inv.Head
	}
	ver, exists := inv.Versions[num]
	if !exists {
		return nil
	}
	allPaths := ver.State.AllPaths()
	state := &ocfl.VState{
		State:   make(map[string][]string, len(allPaths)),
		Created: ver.Created,
		Message: ver.Message,
	}
	if ver.User != nil {
		state.User.Address = ver.User.Address
		state.User.Name = ver.User.Name
	}
	for p, d := range allPaths {
		state.State[p] = inv.Manifest.DigestPaths(d)
	}
	return state
}

// ContentPath returns the content path for the logical path present in the
// state for version vnum. The content path is relative to the object's root
// directory (i.e, as it appears in the inventory manifest). If vnum is empty,
// the inventories head version is used.
func (inv *Inventory) ContentPath(vnum ocfl.VNum, logical string) (string, error) {
	if vnum.Empty() {
		vnum = inv.Head
	}
	vstate := inv.VState(vnum)
	if vstate == nil {
		return "", fmt.Errorf("version doesn't exist in inventory: %s", vnum)
	}
	paths, exists := vstate.State[logical]
	if !exists {
		return "", fmt.Errorf("logical path doesn't exist in inventory %s: %s", vnum, logical)
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("BUG: %s: %s VState is empty slice", vnum, logical)
	}
	return paths[0], nil
}

// WriteInventory marshals the value pointed to by inv, writing the json to dir/inventory.json in
// fsys. The digest is calculated using alg and the inventory sidecar is also writen to
// dir/inventory.alg
func WriteInventory(ctx context.Context, fsys backend.Writer, dir string, inv *Inventory) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	checksum := inv.DigestAlgorithm.New()
	byt, err := json.MarshalIndent(inv, "", " ")
	if err != nil {
		return err
	}
	_, err = io.Copy(checksum, bytes.NewBuffer(byt))
	if err != nil {
		return err
	}
	sum := hex.EncodeToString(checksum.Sum(nil))
	// write inventory.json and sidecar
	invFile := path.Join(dir, inventoryFile)
	sideFile := invFile + "." + inv.DigestAlgorithm.ID()
	_, err = fsys.Write(invFile, bytes.NewBuffer(byt))
	if err != nil {
		return fmt.Errorf("write inventory failed: %w", err)
	}
	_, err = fsys.Write(sideFile, strings.NewReader(sum+" "+inventoryFile+"\n"))
	if err != nil {
		return fmt.Errorf("write inventory sidecar failed: %w", err)
	}
	return nil
}

// readInventorySidecar parses the contents of file as an inventory sidecar, returning
// the stored digest on succecss. An error is returned if the sidecar is not in the expected
// format
func readInventorySidecar(ctx context.Context, file io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
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
