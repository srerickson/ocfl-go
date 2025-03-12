package ocfl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

var (
	// Error: invalid contents of inventory sidecar file
	ErrInventorySidecarContents = errors.New("invalid contents of inventory sidecar file")

	invSidecarContentsRexp = regexp.MustCompile(`^([a-fA-F0-9]+)\s+inventory\.json[\n]?$`)
)

type Inventory interface {
	FixitySource
	ContentDirectory() string
	Digest() string
	DigestAlgorithm() digest.Algorithm
	Head() VNum
	ID() string
	Manifest() DigestMap
	Spec() Spec
	Version(int) ObjectVersion
	FixityAlgorithms() []string
}

type ObjectVersion interface {
	State() DigestMap
	User() *User
	Message() string
	Created() time.Time
}

// User is a generic user information struct
type User struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}

// ReadInventory reads the 'inventory.json' file in dir and validates it. It returns
// an error if the inventory cann't be paresed or if it is invalid.
func ReadInventory(ctx context.Context, fsys ocflfs.FS, dir string) (inv Inventory, err error) {
	var byts []byte
	var imp ocfl
	byts, err = ocflfs.ReadAll(ctx, fsys, path.Join(dir, inventoryBase))
	if err != nil {
		return
	}
	imp, err = getInventoryOCFL(byts)
	if err != nil {
		return
	}
	return imp.NewInventory(byts)
}

// ReadSidecarDigest reads the digest from an inventory.json sidecar file
func ReadSidecarDigest(ctx context.Context, fsys ocflfs.FS, name string) (string, error) {
	byts, err := ocflfs.ReadAll(ctx, fsys, name)
	if err != nil {
		return "", err
	}
	matches := invSidecarContentsRexp.FindSubmatch(byts)
	if len(matches) != 2 {
		err := fmt.Errorf("reading %s: %w", name, ErrInventorySidecarContents)
		return "", err
	}
	return string(matches[1]), nil
}

// ValidateInventoryBytes parses and fully validates the byts as contents of an
// inventory.json file. This is mostly used for testing.
func ValidateInventoryBytes(byts []byte) (Inventory, *Validation) {
	imp, _ := getInventoryOCFL(byts)
	if imp == nil {
		// use default OCFL spec
		imp = defaultOCFL()
	}
	return imp.ValidateInventoryBytes(byts)
}

// ValidateInventorySidecar reads the inventory sidecar with inv's digest
// algorithm (e.g., inventory.json.sha512) in directory dir and return an error
// if the sidecar content is not formatted correctly or if the inv's digest
// doesn't match the value found in the sidecar.
func ValidateInventorySidecar(ctx context.Context, inv Inventory, fsys ocflfs.FS, dir string) error {
	sideCar := path.Join(dir, inventoryBase+"."+inv.DigestAlgorithm().ID())
	expSum, err := ReadSidecarDigest(ctx, fsys, sideCar)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expSum, inv.Digest()) {
		return &digest.DigestError{
			Path:     sideCar,
			Alg:      inv.DigestAlgorithm().ID(),
			Got:      inv.Digest(),
			Expected: expSum,
		}
	}
	return nil
}

func validateInventory(inv Inventory) *Validation {
	imp, err := getOCFL(inv.Spec())
	if err != nil {
		v := &Validation{}
		err := fmt.Errorf("inventory has invalid 'type':%w", err)
		v.AddFatal(err)
		return v
	}
	return imp.ValidateInventory(inv)
}

// get the ocfl implementation declared in the inventory bytes
func getInventoryOCFL(byts []byte) (ocfl, error) {
	invFields := struct {
		Type InventoryType `json:"type"`
	}{}
	if err := json.Unmarshal(byts, &invFields); err != nil {
		return nil, err
	}
	return getOCFL(invFields.Type.Spec)
}

// rawInventory represents the contents of an object's inventory.json file
type rawInventory struct {
	ID               string                        `json:"id"`
	Type             InventoryType                 `json:"type"`
	DigestAlgorithm  string                        `json:"digestAlgorithm"`
	Head             VNum                          `json:"head"`
	ContentDirectory string                        `json:"contentDirectory,omitempty"`
	Manifest         DigestMap                     `json:"manifest"`
	Versions         map[VNum]*rawInventoryVersion `json:"versions"`
	Fixity           map[string]DigestMap          `json:"fixity,omitempty"`
}

func (inv rawInventory) getFixity(dig string) digest.Set {
	paths := inv.Manifest[dig]
	if len(paths) < 1 {
		return nil
	}
	set := digest.Set{}
	for fixAlg, fixMap := range inv.Fixity {
		for p, fixDigest := range fixMap.Paths() {
			if slices.Contains(paths, p) {
				set[fixAlg] = fixDigest
				break
			}
		}
	}
	return set
}

func (inv rawInventory) version(v int) *rawInventoryVersion {
	if inv.Versions == nil {
		return nil
	}
	if v == 0 {
		return inv.Versions[inv.Head]
	}
	vnum := V(v, inv.Head.Padding())
	return inv.Versions[vnum]
}

// vnums returns a sorted slice of vnums corresponding to the keys in the
// inventory's 'versions' block.
func (inv rawInventory) vnums() []VNum {
	vnums := make([]VNum, len(inv.Versions))
	i := 0
	for v := range inv.Versions {
		vnums[i] = v
		i++
	}
	sort.Sort(VNums(vnums))
	return vnums
}

// Version represents object version state and metadata
type rawInventoryVersion struct {
	Created time.Time `json:"created"`
	State   DigestMap `json:"state"`
	Message string    `json:"message,omitempty"`
	User    *User     `json:"user,omitempty"`
}
