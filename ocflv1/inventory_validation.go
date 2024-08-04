package ocflv1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1/codes"
	"golang.org/x/exp/maps"
)

// Validate validates the inventory. It only checks the inventory's structure
// and internal consistency. The inventory is valid if the returned validation
// result includes no fatal errors (it may include warning errors). The returned
// validation.Result is not associated with a logger, and no errors in the result
// have been logged.
func (inv *RawInventory) Validate(opts ...ocfl.ValidationOption) *ocfl.Validation {
	result := ocfl.NewValidation(opts...)
	if inv.Type.Empty() {
		err := errors.New("missing required field: 'type'")
		result.AddFatal(err)
	}
	ocflV := inv.Type.Spec
	if inv.ID == "" {
		err := errors.New("missing required field: 'id'")
		result.AddFatal(ec(err, codes.E036(ocflV)))
	}
	if inv.Head.IsZero() {
		err := errors.New("missing required field: 'head'")
		result.AddFatal(ec(err, codes.E036(ocflV)))
	}
	if inv.Manifest == nil {
		err := errors.New("missing required field 'manifest'")
		result.AddFatal(ec(err, codes.E041(ocflV)))
	}
	if inv.Versions == nil {
		err := errors.New("missing required field 'versions'")
		result.AddFatal(ec(err, codes.E041(ocflV)))
	}
	// don't continue if there are fatal errors
	if result.Err() != nil {
		return result
	}
	if u, err := url.ParseRequestURI(inv.ID); err != nil || u.Scheme == "" {
		err := fmt.Errorf(`object ID is not a URI: %s`, inv.ID)
		result.AddWarn(ec(err, codes.W005(ocflV)))
	}
	switch inv.DigestAlgorithm {
	case ocfl.SHA512:
		break
	case ocfl.SHA256:
		err := fmt.Errorf(`'digestAlgorithm' is %q`, ocfl.SHA256)
		result.AddWarn(ec(err, codes.W004(ocflV)))
	default:
		err := fmt.Errorf(`'digestAlgorithm' is not %q or %q`, ocfl.SHA512, ocfl.SHA256)
		result.AddFatal(ec(err, codes.E025(ocflV)))
	}
	if err := inv.Head.Valid(); err != nil {
		// this shouldn't ever trigger since the invalid condition is caught during unmarshal.
		err = fmt.Errorf("head is invalid: %w", err)
		result.AddFatal(ec(err, codes.E011(ocflV)))
	}
	if strings.Contains(inv.ContentDirectory, "/") {
		err := errors.New("contentDirectory contains '/'")
		result.AddFatal(ec(err, codes.E017(ocflV)))
	}
	if inv.ContentDirectory == "." || inv.ContentDirectory == ".." {
		err := errors.New("contentDirectory is '.' or '..'")
		result.AddFatal(ec(err, codes.E017(ocflV)))
	}
	if err := inv.Manifest.Valid(); err != nil {
		var dcErr *ocfl.MapDigestConflictErr
		var pcErr *ocfl.MapPathConflictErr
		var piErr *ocfl.MapPathInvalidErr
		if errors.As(err, &dcErr) {
			err = ec(err, codes.E096(ocflV))
		} else if errors.As(err, &pcErr) {
			err = ec(err, codes.E101(ocflV))
		} else if errors.As(err, &piErr) {
			err = ec(err, codes.E099(ocflV))
		}
		result.AddFatal(err)
	}
	// version names
	var versionNums ocfl.VNums = maps.Keys(inv.Versions)
	if err := versionNums.Valid(); err != nil {
		if errors.Is(err, ocfl.ErrVerEmpty) {
			err = ec(err, codes.E008(ocflV))
		} else if errors.Is(err, ocfl.ErrVNumMissing) {
			err = ec(err, codes.E010(ocflV))
		} else if errors.Is(err, ocfl.ErrVNumPadding) {
			err = ec(err, codes.E012(ocflV))
		}
		result.AddFatal(err)
	}
	if versionNums.Head() != inv.Head {
		err := fmt.Errorf(`version head not most recent version: %s`, inv.Head)
		result.AddFatal(ec(err, codes.E040(ocflV)))
	}
	// version state
	for vname, ver := range inv.Versions {
		if ver == nil {
			err := fmt.Errorf(`missing required version block for %q`, vname)
			result.AddFatal(ec(err, codes.E048(ocflV)))
			continue
		}
		if ver.Created.IsZero() {
			err := fmt.Errorf(`version %s missing required field: 'created'`, vname)
			result.AddFatal(ec(err, codes.E048(ocflV)))
		}
		if ver.Message == "" {
			err := fmt.Errorf("version %s missing recommended field: 'message'", vname)
			result.AddWarn(ec(err, codes.W007(ocflV)))
		}
		if ver.User != nil {
			if ver.User.Name == "" {
				err := fmt.Errorf("version %s user missing required field: 'name'", vname)
				result.AddFatal(ec(err, codes.E054(ocflV)))
			}
			if ver.User.Address == "" {
				err := fmt.Errorf("version %s user missing recommended field: 'address'", vname)
				result.AddWarn(ec(err, codes.W008(ocflV)))
			}
			if u, err := url.ParseRequestURI(ver.User.Address); err != nil || u.Scheme == "" {
				err := fmt.Errorf("version %s user address is not a URI", vname)
				result.AddWarn(ec(err, codes.W009(ocflV)))
			}
		}
		if ver.State == nil {
			err := fmt.Errorf(`version %s missing required field: 'state'`, vname)
			result.AddFatal(ec(err, codes.E048(ocflV)))
			continue
		}
		err := ver.State.Valid()
		if err != nil {
			var dcErr *ocfl.MapDigestConflictErr
			var pcErr *ocfl.MapPathConflictErr
			var piErr *ocfl.MapPathInvalidErr
			if errors.As(err, &dcErr) {
				err = ec(err, codes.E050(ocflV))
			} else if errors.As(err, &pcErr) {
				err = ec(err, codes.E095(ocflV))
			} else if errors.As(err, &piErr) {
				err = ec(err, codes.E052(ocflV))
			}
			result.AddFatal(err)
		}
		// check that each state digest appears in manifest
		for _, digest := range ver.State.Digests() {
			if len(inv.Manifest[digest]) == 0 {
				err := fmt.Errorf("digest in %s state not in manifest: %s", vname, digest)
				result.AddFatal(ec(err, codes.E050(ocflV)))
			}
		}
	}
	// check that each manifest entry is used in at least one state
	for _, digest := range inv.Manifest.Digests() {
		var found bool
		for _, version := range inv.Versions {
			if len(version.State[digest]) > 0 {
				found = true
				break
			}
		}
		if !found {
			err := fmt.Errorf("digest in manifest not used in version state: %s", digest)
			result.AddFatal(ec(err, codes.E107(ocflV)))
		}
	}
	//fixity
	for _, fixity := range inv.Fixity {
		err := fixity.Valid()
		if err != nil {
			var dcErr *ocfl.MapDigestConflictErr
			var piErr *ocfl.MapPathInvalidErr
			var pcErr *ocfl.MapPathConflictErr
			if errors.As(err, &dcErr) {
				err = ec(err, codes.E097(ocflV))
			} else if errors.As(err, &piErr) {
				err = ec(err, codes.E099(ocflV))
			} else if errors.As(err, &pcErr) {
				err = ec(err, codes.E101(ocflV))
			}
			result.AddFatal(err)
		}
	}
	return result
}

// ValidateInventory fully validates an inventory at path name in fsys.
func ValidateInventory(ctx context.Context, fsys ocfl.FS, name string, ocflV ocfl.Spec) (inv *RawInventory, result *ocfl.Validation) {
	f, err := fsys.OpenFile(ctx, name)
	if err != nil {
		result.AddFatal(err)
		return nil, result
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			result.AddFatal(closeErr)
		}
	}()
	inv, result = ValidateInventoryReader(ctx, f)
	if result.Err() != nil {
		return
	}
	ocflV = inv.Type.Spec
	side := name + "." + inv.DigestAlgorithm
	expSum, err := readInventorySidecar(ctx, fsys, side)
	if err != nil {
		if errors.Is(err, ErrInvSidecarContents) {
			result.AddFatal(ec(err, codes.E061(ocflV)))
			return
		}
		result.AddFatal(ec(err, codes.E058(ocflV)))
		return
	}
	if !strings.EqualFold(inv.jsonDigest, expSum) {
		shortSum := inv.jsonDigest[:6]
		shortExp := expSum[:6]
		err := fmt.Errorf("inventory's checksum (%s) doen't match expected value in sidecar (%s): %s", shortSum, shortExp, name)
		result.AddFatal(ec(err, codes.E060(ocflV)))
	}
	return
}

func ValidateInventoryReader(ctx context.Context, reader io.Reader) (*RawInventory, *ocfl.Validation) {
	inv, err := readDigestInventory(ctx, reader)
	if err != nil {
		result := &ocfl.Validation{}
		result.AddFatal(err)
		return nil, result
	}
	return inv, inv.Validate()
}

// readDigestInventory reads the inventory and sets its digest value using
// the digest algorithm
func readDigestInventory(ctx context.Context, reader io.Reader) (*RawInventory, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	byt, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(byt))
	dec.DisallowUnknownFields()
	inv := &RawInventory{}
	if err := dec.Decode(inv); err != nil {
		return nil, err
	}
	digester := ocfl.NewDigester(inv.DigestAlgorithm)
	if digester == nil {
		return nil, fmt.Errorf("%w: %q", ocfl.ErrUnknownAlg, inv.DigestAlgorithm)
	}
	if _, err := io.Copy(digester, bytes.NewReader(byt)); err != nil {
		return nil, err
	}
	inv.jsonDigest = digester.String()
	return inv, nil
}
