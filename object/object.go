package object

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"path"
	"regexp"
	"strings"

	"github.com/srerickson/ocfl/backend"
	"github.com/srerickson/ocfl/digest"
)

const (
	ocflObject    = "ocfl_object"
	inventoryFile = "inventory.json"
)

var (
	ErrNotObject          = errors.New("not an OCFL object")
	ErrOCFLVersion        = errors.New("unsupported OCFL version")
	ErrInventoryOpen      = errors.New("could not read inventory file")
	ErrInvSidecarOpen     = errors.New("could not read inventory sidecar file")
	ErrInvSidecarContents = errors.New("invalid inventory sidecar contents")
	ErrInvSidecarChecksum = errors.New("inventory digest doesn't match expected value from sidecar file")
	ErrDigestAlg          = errors.New("invalid digest algorithm")
	ErrObjRootStructure   = errors.New("object includes invalid files or directories")

	invSidecarContentsRexp = regexp.MustCompile(`^([a-fA-F0-9]+)\s+inventory\.json[\n]?$`)
)

// Write marshals the value pointed to by inv, writing the json to dir/inventory.json in
// fsys. The digest is calculated using alg and the inventory sidecar is also writen to
// dir/inventory.alg
func WriteInventory(ctx context.Context, fsys backend.Writer, dir string, alg digest.Alg, inv interface{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	checksum := alg.New()
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
	sideFile := fmt.Sprintf("%s.%s", invFile, alg)
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

// ReadDigestInventory reads decodes the contents of file into the value pointed to by inv; it also
// digests the contents of the reader using the digest algorithm alg, returning the digest string.
func ReadDigestInventory(ctx context.Context, file io.Reader, inv interface{}, alg digest.Alg) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if alg.ID() != "" {
		checksum := alg.New()
		reader := io.TeeReader(file, checksum)
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := json.NewDecoder(reader).Decode(inv); err != nil {
			return "", err
		}
		return hex.EncodeToString(checksum.Sum(nil)), nil
	}

	// otherwise, need to decode inventory twice
	byt, err := ioutil.ReadAll(file)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(byt, inv); err != nil {
		return "", err
	}
	// decode to get digestAlgorithm
	var tmpInv struct {
		Digest digest.Alg `json:"digestAlgorithm"`
	}
	if err = json.Unmarshal(byt, &tmpInv); err != nil {
		return "", err
	}
	checksum := tmpInv.Digest.New()
	_, err = checksum.Write(byt)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(checksum.Sum(nil)), nil
}

// ReadInventorySidecar parses the contents of file as an inventory sidecar, returning
// the stored digest on succecss. An error is returned if the sidecar is not in the expected
// format
func ReadInventorySidecar(ctx context.Context, file io.Reader) (string, error) {
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
