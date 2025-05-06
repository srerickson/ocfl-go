package ocfl

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

	"github.com/srerickson/ocfl-go/digest"
	ocflfs "github.com/srerickson/ocfl-go/fs"
)

var (
	// ErrInventorySidecarContents indicates the inventory sidecar contents is
	// not formatted correctly.
	ErrInventorySidecarContents = errors.New("invalid contents of inventory sidecar file")

	invSidecarContentsRexp = regexp.MustCompile(`^([a-fA-F0-9]+)\s+inventory\.json[\n]?$`)
)

// Inventory represents the contents of an object's inventory.json file
type Inventory struct {
	ID               string                     `json:"id"`
	Type             InventoryType              `json:"type"`
	DigestAlgorithm  string                     `json:"digestAlgorithm"`
	Head             VNum                       `json:"head"`
	ContentDirectory string                     `json:"contentDirectory,omitempty"`
	Manifest         DigestMap                  `json:"manifest"`
	Versions         map[VNum]*InventoryVersion `json:"versions"`
	Fixity           map[string]DigestMap       `json:"fixity,omitempty"`

	jsonDigest string
}

func (inv Inventory) GetFixity(dig string) digest.Set {
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

// raw of the inventory.json file the inventory was read from. Only set if the
// digest was read from a file.
func (inv Inventory) Digest() string {
	return inv.jsonDigest
}

func (inv *Inventory) Validate() *Validation {
	imp, err := getOCFL(inv.Type.Spec)
	if err != nil {
		v := &Validation{}
		err := fmt.Errorf("inventory has invalid 'type':%w", err)
		v.AddFatal(err)
		return v
	}
	return imp.ValidateInventory(inv)
}

func (inv *Inventory) setJsonDigest(raw []byte) error {
	digester, err := digest.DefaultRegistry().NewDigester(inv.DigestAlgorithm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(digester, bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("digesting inventory: %w", err)
	}
	inv.jsonDigest = digester.String()
	return nil
}

func (inv Inventory) version(v int) *InventoryVersion {
	if inv.Versions == nil {
		return nil
	}
	if v < 1 {
		return inv.Versions[inv.Head]
	}
	return inv.Versions[V(v, inv.Head.padding)]
}

// vnums returns a sorted slice of vnums corresponding to the keys in the
// inventory's 'versions' block.
func (inv Inventory) vnums() []VNum {
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
type InventoryVersion struct {
	Created time.Time `json:"created"`
	State   DigestMap `json:"state"`
	Message string    `json:"message,omitempty"`
	User    *User     `json:"user,omitempty"`
}

// User is a generic user information struct
type User struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}

// ReadInventory reads the 'inventory.json' file in dir and validates it. It returns
// an error if the inventory can't be paresed or if it is invalid.
func ReadInventory(ctx context.Context, fsys ocflfs.FS, dir string) (inv *Inventory, err error) {
	var byts []byte
	byts, err = ocflfs.ReadAll(ctx, fsys, path.Join(dir, inventoryBase))
	if err != nil {
		return
	}
	inv = &Inventory{}
	dec := json.NewDecoder(bytes.NewReader(byts))
	dec.DisallowUnknownFields()
	if err := dec.Decode(inv); err != nil {
		return nil, err
	}
	if err := inv.setJsonDigest(byts); err != nil {
		return nil, err
	}
	if err := inv.Validate().Err(); err != nil {
		return nil, err
	}
	return inv, nil
}

// ReadInventorySidecar reads the digest from an inventory sidecar file in
// dir, using the digest algorithm alg.
func ReadInventorySidecar(ctx context.Context, fsys ocflfs.FS, dir, alg string) (string, error) {
	sideCar := path.Join(dir, inventoryBase+"."+alg)
	byts, err := ocflfs.ReadAll(ctx, fsys, sideCar)
	if err != nil {
		return "", err
	}
	matches := invSidecarContentsRexp.FindSubmatch(byts)
	if len(matches) != 2 {
		err := fmt.Errorf("reading %s: %w", sideCar, ErrInventorySidecarContents)
		return "", err
	}
	return string(matches[1]), nil
}

// ValidateInventoryBytes parses and fully validates the byts as contents of an
// inventory.json file. This is mostly used for testing.
func ValidateInventoryBytes(byts []byte) (*Inventory, *Validation) {
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
func ValidateInventorySidecar(ctx context.Context, inv *Inventory, fsys ocflfs.FS, dir string) error {
	sideCar := path.Join(dir, inventoryBase+"."+inv.DigestAlgorithm)
	expSum, err := ReadInventorySidecar(ctx, fsys, dir, inv.DigestAlgorithm)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expSum, inv.Digest()) {
		return &digest.DigestError{
			Path:     sideCar,
			Alg:      inv.DigestAlgorithm,
			Got:      inv.Digest(),
			Expected: expSum,
		}
	}
	return nil
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

type InventoryBuilder struct {
	prev *Inventory

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
	contentPath PathMutation
	fixtySource FixitySource
}

// Create a new inventory builder. If prev is not nil, the builder's initial
// Spec, ID, and ContentDirectory are used.
func NewInventoryBuilder(prev *Inventory) *InventoryBuilder {
	b := &InventoryBuilder{
		spec: Spec1_1, // default inventory specification
		prev: prev,
	}
	if prev != nil {
		b.id = prev.ID
		b.spec = prev.Type.Spec
		b.contDir = prev.ContentDirectory
		b.head = prev.Head
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
func (b *InventoryBuilder) ContentPathFunc(mutate PathMutation) *InventoryBuilder {
	b.contentPath = mutate
	return b
}

// Finalize builds and validates a new inventory.
func (b *InventoryBuilder) Finalize() (*Inventory, error) {
	newInv, err := b.initialInventory()
	if err != nil {
		return nil, err
	}
	if err := b.buildVersions(newInv); err != nil {
		return nil, err
	}
	b.fillFixity(newInv)
	if err := newInv.Validate().Err(); err != nil {
		return nil, fmt.Errorf("generated inventory is not valid: %w", err)
	}
	return newInv, nil
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

func (b *InventoryBuilder) initialInventory() (*Inventory, error) {
	inv := &Inventory{
		ID:               b.id,
		Head:             b.head,
		Type:             b.spec.InventoryType(),
		ContentDirectory: b.contDir,
		Manifest:         DigestMap{},
		Fixity:           map[string]DigestMap{},
		Versions:         map[VNum]*InventoryVersion{},
	}
	if b.prev == nil {
		return inv, nil
	}
	// copy manifest
	inv.DigestAlgorithm = b.prev.DigestAlgorithm
	var err error
	inv.Manifest, err = b.prev.Manifest.Normalize()
	if err != nil {
		return nil, fmt.Errorf("in existing inventory manifest: %w", err)
	}
	// copy versions
	versions := b.prev.Head.Lineage()
	inv.Versions = make(map[VNum]*InventoryVersion, len(versions)+1)
	for vnum, prevVer := range b.prev.Versions {
		newVer := &InventoryVersion{
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
	inv.Fixity = make(map[string]DigestMap, len(b.prev.Fixity))
	for alg, m := range b.prev.Fixity {
		inv.Fixity[alg], err = m.Normalize()
		if err != nil {
			return nil, fmt.Errorf("in existing inventory %s fixity: %w", alg, err)
		}
	}
	return inv, nil
}

func (b *InventoryBuilder) buildVersions(inv *Inventory) error {
	for _, versionInput := range b.addedVersions {
		newHead, err := inv.Head.Next()
		if err != nil {
			return fmt.Errorf("existing inventory's version scheme doesn't support additional versions: %w", err)
		}
		newState, err := versionInput.state.Normalize()
		if err != nil {
			return fmt.Errorf("%s version state: %w", newHead, err)
		}
		alg := versionInput.alg
		if inv.DigestAlgorithm == "" {
			inv.DigestAlgorithm = alg.ID()
		}
		if inv.DigestAlgorithm != alg.ID() {
			return fmt.Errorf("cannot change inventory's digest algorithm from previous value: %s", inv.DigestAlgorithm)
		}
		newVersion := &InventoryVersion{
			State:   newState,
			Created: versionInput.created,
			Message: versionInput.messsage,
			User:    versionInput.user,
		}
		if newVersion.Created.IsZero() {
			newVersion.Created = time.Now()
		}
		newVersion.Created = newVersion.Created.Truncate(time.Second)
		if inv.Versions == nil {
			inv.Versions = map[VNum]*InventoryVersion{}
		}
		inv.Head = newHead
		inv.Versions[newHead] = newVersion
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
		for digest, logicPaths := range newState {
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
func (b *InventoryBuilder) fillFixity(inv *Inventory) {
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
