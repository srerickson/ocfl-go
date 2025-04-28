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

type InventoryBuilder struct {
	prev Inventory

	// set by builder methodss
	id            string
	contDir       string
	head          VNum
	spec          Spec
	addedVersions []struct {
		state    DigestMap
		alg      digest.Algorithm
		created  time.Time
		messsage string
		user     *User
	}
	contentPath func([]string) []string
	fixtySource FixitySource
}

// Create a new inventory builder. If prev is not nil, the builder's initial
// Spec, ID, and ContentDirectory are used.
func NewInventoryBuilder(prev Inventory) *InventoryBuilder {
	b := &InventoryBuilder{
		spec: Spec1_1, // default inventory specification
		prev: prev,
	}
	if prev != nil {
		b.id = prev.ID()
		b.spec = prev.Spec()
		b.contDir = prev.ContentDirectory()
		b.head = prev.Head()
	}
	return b
}

// AddVersion increments the inventory's head and adds a new version with the
// given stage, creation timestamp, message, and user information.
func (b *InventoryBuilder) AddVersion(state DigestMap, alg digest.Algorithm, created time.Time, message string, user *User) *InventoryBuilder {
	added := struct {
		state    DigestMap
		alg      digest.Algorithm
		created  time.Time
		messsage string
		user     *User
	}{
		state:    state,
		alg:      alg,
		created:  created,
		messsage: message,
		user:     user,
	}
	b.addedVersions = append(b.addedVersions, added)
	return b
}

// Sets the inventory's content directory. Ignored if the builder was
// initialized with an existing inventory.
func (b *InventoryBuilder) ContentDirectory(name string) *InventoryBuilder {
	if b.prev == nil {
		b.contDir = name
	}
	return b
}

// ContentPathFunc sets a function used to generate content directory names for
// new manifest entries.
func (b *InventoryBuilder) ContentPathFunc(f func([]string) []string) *InventoryBuilder {
	b.contentPath = f
	return b
}

// Finalize builds and validates a new inventory.
func (b *InventoryBuilder) Finalize() (Inventory, error) {
	newInv, err := b.initialInventory()
	if err != nil {
		return nil, err
	}
	if err := b.buildVersions(newInv); err != nil {
		return nil, err
	}
	b.fillFixity(newInv)
	//FIXME
	v1Inv := &inventoryV1{raw: *newInv}
	if err := validateInventory(v1Inv).Err(); err != nil {
		return nil, fmt.Errorf("generated inventory is not valid: %w", err)
	}
	return v1Inv, nil
}

func (b *InventoryBuilder) FixitySource(source FixitySource) *InventoryBuilder {
	b.fixtySource = source
	return b
}

// ID sets the inventory's ID
func (b *InventoryBuilder) ID(id string) *InventoryBuilder {
	b.id = id
	return b
}

// Padding sets the inventory's version number padding. Ignored if the builder
// was initialized with an existing inventory.
func (b *InventoryBuilder) Padding(p int) *InventoryBuilder {
	if b.prev == nil {
		b.head = V(0, p)
	}
	return b
}

// Set the inventory's OCFL spec. Ignored if spec is a zero value.
func (b *InventoryBuilder) Spec(spec Spec) *InventoryBuilder {
	if spec.Empty() {
		return b
	}
	b.spec = spec
	return b
}

func (b *InventoryBuilder) initialInventory() (*rawInventory, error) {
	inv := &rawInventory{
		ID:               b.id,
		Head:             b.head,
		Type:             b.spec.InventoryType(),
		ContentDirectory: b.contDir,
		Manifest:         DigestMap{},
		Fixity:           map[string]DigestMap{},
		Versions:         map[VNum]*rawInventoryVersion{},
	}
	if b.prev == nil {
		return inv, nil
	}
	prevInv, ok := b.prev.(*inventoryV1)
	if !ok {
		return nil, errors.New("previous inventory does not have expected type")
	}
	// copy manifest
	inv.DigestAlgorithm = prevInv.raw.DigestAlgorithm
	var err error
	inv.Manifest, err = prevInv.raw.Manifest.Normalize()
	if err != nil {
		return nil, fmt.Errorf("in existing inventory manifest: %w", err)
	}
	// copy versions
	versions := prevInv.raw.Head.Lineage()
	inv.Versions = make(map[VNum]*rawInventoryVersion, len(versions)+1)
	for vnum, prevVer := range prevInv.raw.Versions {
		newVer := &rawInventoryVersion{
			Created: prevVer.Created,
			Message: prevVer.Message,
		}
		newVer.State, err = prevVer.State.Normalize()
		if err != nil {
			return nil, fmt.Errorf("in existing inventory %s state: %w", vnum, err)
		}
		if prevVer.User != nil {
			newVer.User = &User{}
			*newVer.User = *prevVer.User
		}
		inv.Versions[vnum] = newVer
	}
	// copy fixity
	inv.Fixity = make(map[string]DigestMap, len(prevInv.raw.Fixity))
	for alg, m := range prevInv.raw.Fixity {
		inv.Fixity[alg], err = m.Normalize()
		if err != nil {
			return nil, fmt.Errorf("in existing inventory %s fixity: %w", alg, err)
		}
	}
	return inv, nil
}

func (b *InventoryBuilder) buildVersions(inv *rawInventory) error {
	for _, addedVer := range b.addedVersions {
		newHead, err := inv.Head.Next()
		if err != nil {
			return fmt.Errorf("existing inventory's version scheme doesn't support additional versions: %w", err)
		}
		state, err := addedVer.state.Normalize()
		if err != nil {
			return fmt.Errorf("%s version state: %w", newHead, err)
		}
		alg := addedVer.alg
		message := addedVer.messsage
		user := addedVer.user
		created := addedVer.created

		if inv.DigestAlgorithm == "" {
			inv.DigestAlgorithm = alg.ID()
		}
		if inv.DigestAlgorithm != alg.ID() {
			return fmt.Errorf("cannot change inventory's digest algorithm from previous value: %s", inv.DigestAlgorithm)
		}
		inv.Head = newHead
		newVersion := &rawInventoryVersion{
			State:   state,
			Created: created,
			Message: message,
			User:    user,
		}
		if newVersion.Created.IsZero() {
			newVersion.Created = time.Now()
		}
		newVersion.Created = newVersion.Created.Truncate(time.Second)
		if inv.Versions == nil {
			inv.Versions = map[VNum]*rawInventoryVersion{}
		}
		inv.Versions[inv.Head] = newVersion
		// add version state to manifest
		contentDirectory := inv.ContentDirectory
		if contentDirectory == "" {
			contentDirectory = contentDir
		}
		contentPathFunc := func(paths []string) []string {
			paths = slices.Clone(paths)
			// apply user-specified path transform first
			if b.contentPath != nil {
				paths = b.contentPath(paths)
			}
			// build version's content paths from logical paths
			for i, p := range paths {
				paths[i] = path.Join(newHead.String(), contentDirectory, p)
			}
			return paths
		}
		for digest, logicPaths := range state {
			if len(inv.Manifest[digest]) > 0 {
				continue // version content already exists in the manifest
			}
			inv.Manifest[digest] = contentPathFunc(logicPaths)
		}
	}
	return nil

}

// fillFixity adds fixity entries from source using for all digests found in the
// inventory's manifest.
func (b *InventoryBuilder) fillFixity(inv *rawInventory) {
	if b.fixtySource == nil {
		return
	}
	for digest, contentPaths := range inv.Manifest {
		fixSet := b.fixtySource.GetFixity(digest)
		if len(fixSet) < 1 {
			continue
		}
		for fixAlg, fixDigest := range fixSet {
			if inv.Fixity[fixAlg] == nil {
				inv.Fixity[fixAlg] = DigestMap{}
			}
			for _, cp := range contentPaths {
				fixPaths := inv.Fixity[fixAlg][fixDigest]
				if !slices.Contains(fixPaths, cp) {
					inv.Fixity[fixAlg][fixDigest] = append(fixPaths, cp)
				}
			}
		}
	}
}
