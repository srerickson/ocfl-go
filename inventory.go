package ocfl

import (
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

	"github.com/srerickson/ocfl-go/digest"
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
	Validate() *Validation
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

func ReadSidecarDigest(ctx context.Context, fsys FS, name string) (digest string, err error) {
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
		err = fmt.Errorf("reading %s: %w", name, ErrInventorySidecarContents)
		return
	}
	digest = string(matches[1])
	return
}

// ValidateInventorySidecar reads the inventory sidecar with inv's digest
// algorithm (e.g., inventory.json.sha512) in directory dir and return an error
// if the sidecar content is not formatted correctly or if the inv's digest
// doesn't match the value found in the sidecar.
func ValidateInventorySidecar(ctx context.Context, inv Inventory, fsys FS, dir string) error {
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

// return a ReadInventory for an inventory that may use any version of the ocfl spec.
func readUnknownInventory(ctx context.Context, ocfls *OCLFRegister, fsys FS, dir string) (Inventory, error) {
	f, err := fsys.OpenFile(ctx, path.Join(dir, inventoryBase))
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	invFields := struct {
		Type InvType `json:"type"`
	}{}
	if err = json.Unmarshal(raw, &invFields); err != nil {
		return nil, err
	}
	invOCFL, err := ocfls.Get(invFields.Type.Spec)
	if err != nil {
		return nil, err
	}
	return invOCFL.NewReadInventory(raw)
}

// rawInventory represents the contents of an object's inventory.json file
type rawInventory struct {
	ID               string                        `json:"id"`
	Type             InvType                       `json:"type"`
	DigestAlgorithm  string                        `json:"digestAlgorithm"`
	Head             VNum                          `json:"head"`
	ContentDirectory string                        `json:"contentDirectory,omitempty"`
	Manifest         DigestMap                     `json:"manifest"`
	Versions         map[VNum]*rawInventoryVersion `json:"versions"`
	Fixity           map[string]DigestMap          `json:"fixity,omitempty"`
}

// getFixity implements ocfl.FixitySource for Inventory
func (inv rawInventory) getFixity(dig string) digest.Set {
	paths := inv.Manifest[dig]
	if len(paths) < 1 {
		return nil
	}
	set := digest.Set{}
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
