package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
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

// func UnmarshalInventory(ctx context.Context, fsys FS, name string, inv ReadInventory) (err error) {
// 	f, err := fsys.OpenFile(ctx, name)
// 	if err != nil {
// 		return
// 	}
// 	defer func() {
// 		if closeErr := f.Close(); closeErr != nil {
// 			err = errors.Join(err, closeErr)
// 		}
// 	}()
// 	raw, err := io.ReadAll(f)
// 	if err != nil {

// 	}

// }
