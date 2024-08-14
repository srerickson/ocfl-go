package ocfl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
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
	DigestAlgorithm() string
	Head() VNum
	ID() string
	Manifest() DigestMap
	Spec() Spec
	// Validate validates the internal structure of the inventory, adding
	// errors and warnings to zero or more validations. The returned
	// error wraps all fatal errors encountered.
	Validate(...*Validation) error
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

func readInventory(ctx context.Context, ocfls *OCLFRegister, fsys FS, name string) (ReadInventory, error) {
	f, err := fsys.OpenFile(ctx, name)
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
	inv, err := invOCFL.NewReadInventory(raw)
	if err != nil {
		return nil, err
	}
	expSum, err := ReadSidecarDigest(ctx, fsys, name+"."+inv.DigestAlgorithm())
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(expSum, inv.Digest()) {
		err := &DigestError{
			Name:     name,
			Alg:      inv.DigestAlgorithm(),
			Got:      inv.Digest(),
			Expected: expSum,
		}
		return nil, err
	}
	return inv, nil
}
