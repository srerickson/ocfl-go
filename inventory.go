package ocfl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/srerickson/ocfl-go/digest"
)

var (
	// Error: invalid contents of inventory sidecar file
	ErrInventorySidecarContents = errors.New("invalid contents of inventory sidecar file")

	invSidecarContentsRexp = regexp.MustCompile(`^([a-fA-F0-9]+)\s+inventory\.json[\n]?$`)
)

type ReadInventory interface {
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
func ValidateInventorySidecar(ctx context.Context, inv ReadInventory, fsys FS, dir string) error {
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
func readUnknownInventory(ctx context.Context, ocfls *OCLFRegister, fsys FS, dir string) (ReadInventory, error) {
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
