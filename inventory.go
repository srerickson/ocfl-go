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
}

// GetFixity provides alternate digests associated with all files with the given
// primary digest.
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

// Validate validates inv using its declarate OCFL spec.
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

// marshal returns the json marshaled inventory with its digest
func (inv Inventory) marshal() ([]byte, string, error) {
	buff, err := json.Marshal(inv)
	if err != nil {
		return nil, "", err
	}
	digester, err := digest.DefaultRegistry().NewDigester(inv.DigestAlgorithm)
	if err != nil {
		return nil, "", err
	}
	if _, err := io.Copy(digester, bytes.NewReader(buff)); err != nil {
		err := fmt.Errorf("digesting inventory: %w", err)
		return nil, "", err
	}
	return buff, digester.String(), nil
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

// versionContent returns a DigestMap with manifest entries
// for content in the specified version.
func (inv Inventory) versionContent(vnum VNum) PathMap {
	prefix := vnum.String() + "/"
	pm := PathMap{}
	for pth, dig := range inv.Manifest.Paths() {
		if strings.HasPrefix(pth, prefix) {
			pm[pth] = dig
		}
	}
	return pm
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

// StoredInventory is an unmarshaled inventory with the digest of its source data.
type StoredInventory struct {
	Inventory

	bytes  []byte // bytes is inventory.json contents
	digest string // digest of bytes using inventory's digest algorithm
}

// NewStoredInventory reads all bytes from r and parses the contents as as
// Inventory. The returned *StoredInventory is valid and includes the a digest
// of the bytes read from r.
func NewStoredInventory(r io.Reader) (*StoredInventory, error) {
	bytes, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading inventory contents: %w", err)
	}
	return newStoredInventory(bytes)
}

// ReadInventory reads the inventory.json file in dir and validates it. It returns
// an error if the inventory can't be parsed or if it is invalid.
func ReadInventory(ctx context.Context, fsys ocflfs.FS, dir string) (*StoredInventory, error) {
	f, err := fsys.OpenFile(ctx, path.Join(dir, inventoryBase))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return NewStoredInventory(f)
}

func newStoredInventory(raw []byte) (*StoredInventory, error) {
	var inv StoredInventory
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&inv); err != nil {
		return nil, err
	}
	if err := inv.setRaw(raw); err != nil {
		return nil, err
	}
	if err := inv.Validate().Err(); err != nil {
		return nil, err
	}
	return &inv, nil
}

// Digest returns the digest of inventory.json file used to create inv using the
// digest algorithm specified in the inventory.
func (inv StoredInventory) Digest() string { return inv.digest }

// MarshalBinary implements [encoding.BinaryMarshaler] for StoredInventory. It
// returns the full contents of the inventory.json file used to create inv.
func (inv StoredInventory) MarshalBinary() ([]byte, error) {
	return bytes.Clone(inv.bytes), nil
}

// UnmarshalBinary implements [encoding.BinaryUnmarshaler] for *StoredInventory.
// The data argument must be the full contents of an inventory.json, otherwise
// inv's Digest value may not match its sidecar. (BinaryUnmarshaler is used
// instead of json.Unmarshaler because the latter may not preserve the exact
// bytestream, for example leading or trailing whitespace.)
func (inv *StoredInventory) UnmarshalBinary(data []byte) error {
	newInv, err := newStoredInventory(data)
	if err != nil {
		return err
	}
	*inv = *newInv
	return nil
}

// ValidateSidecar reads the inventory sidecar with inv's digest
// algorithm (e.g., inventory.json.sha512) in directory dir and return an error
// if the sidecar content is not formatted correctly or if the inv's digest
// doesn't match the value found in the sidecar.
func (inv StoredInventory) ValidateSidecar(ctx context.Context, fsys ocflfs.FS, dir string) error {
	sideCar := path.Join(dir, inventoryBase+"."+inv.DigestAlgorithm)
	expSum, err := ReadInventorySidecar(ctx, fsys, dir, inv.DigestAlgorithm)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expSum, inv.digest) {
		return &digest.DigestError{
			Path:     sideCar,
			Alg:      inv.DigestAlgorithm,
			Got:      inv.digest,
			Expected: expSum,
		}
	}
	return nil
}

// sets inv's jsonDigest value by digesting the raw inventory bytes.
func (inv *StoredInventory) setRaw(raw []byte) error {
	digester, err := digest.DefaultRegistry().NewDigester(inv.DigestAlgorithm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(digester, bytes.NewReader(raw)); err != nil {
		return fmt.Errorf("digesting inventory: %w", err)
	}
	inv.digest = digester.String()
	inv.bytes = raw
	return nil
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
func ValidateInventoryBytes(byts []byte) (*StoredInventory, *Validation) {
	imp, _ := getInventoryOCFL(byts)
	if imp == nil {
		// use default OCFL spec
		imp = defaultOCFL()
	}
	return imp.ValidateInventoryBytes(byts)
}

func writeInventorySidecar(ctx context.Context, fsys ocflfs.FS, dir string, invDigest string, alg string) error {
	sidecarContent := invDigest + " " + inventoryBase + "\n"
	sideFile := path.Join(dir, inventoryBase+"."+alg)
	if _, err := ocflfs.Write(ctx, fsys, sideFile, strings.NewReader(sidecarContent)); err != nil {
		return fmt.Errorf("writing inventory sidecar file: %w", err)
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
