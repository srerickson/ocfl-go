package ocflv1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"strings"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/ocflv1/codes"
	"github.com/srerickson/ocfl-go/validation"
	"golang.org/x/exp/maps"
)

// Validate validates the inventory. It only checks the inventory's structure
// and internal consistency. The inventory is valid if the returned validation
// result includes no fatal errors (it may include warning errors). The returned
// validation.Result is not associated with a logger, and no errors in the result
// have been logged.
func (inv *Inventory) Validate() *validation.Result {
	result := validation.NewResult(-1)
	if inv.Type.Empty() {
		err := errors.New("missing required field: 'type'")
		result.AddFatal(err)
	}
	if inv.ID == "" {
		err := errors.New("missing required field: 'id'")
		result.AddFatal(ec(err, codes.E036.Ref(inv.Type.Spec)))
	}
	if inv.Head.IsZero() {
		err := errors.New("missing required field: 'head'")
		result.AddFatal(ec(err, codes.E036.Ref(inv.Type.Spec)))
	}
	if inv.ContentDirectory == "" {
		inv.ContentDirectory = contentDir
	}
	// don't continue if there are fatal errors
	if result.Err() != nil {
		return result
	}
	if u, err := url.ParseRequestURI(inv.ID); err != nil || u.Scheme == "" {
		err := fmt.Errorf(`object ID is not a URI: %s`, inv.ID)
		result.AddWarn(ec(err, codes.W005.Ref(inv.Type.Spec)))
	}
	switch inv.DigestAlgorithm {
	case ocfl.SHA512:
		break
	case ocfl.SHA256:
		err := fmt.Errorf(`'digestAlgorithm' is %q`, ocfl.SHA256)
		result.AddWarn(ec(err, codes.W004.Ref(inv.Type.Spec)))
	default:
		err := fmt.Errorf(`'digestAlgorithm' is not %q or %q`, ocfl.SHA512, ocfl.SHA256)
		result.AddFatal(ec(err, codes.E025.Ref(inv.Type.Spec)))
	}
	if err := inv.Head.Valid(); err != nil {
		// this shouldn't ever trigger since the invalid condition is caught during unmarshal.
		err = fmt.Errorf("head is invalid: %w", err)
		result.AddFatal(ec(err, codes.E011.Ref(inv.Type.Spec)))
	}
	if strings.Contains(inv.ContentDirectory, "/") {
		err := errors.New("contentDirectory contains '/'")
		result.AddFatal(ec(err, codes.E017.Ref(inv.Type.Spec)))
	}
	if inv.ContentDirectory == "." || inv.ContentDirectory == ".." {
		err := errors.New("contentDirectory is '.' or '..'")
		result.AddFatal(ec(err, codes.E017.Ref(inv.Type.Spec)))
	}
	if err := inv.Manifest.Valid(); err != nil {
		var dcErr *ocfl.MapDigestConflictErr
		var pcErr *ocfl.MapPathConflictErr
		var piErr *ocfl.MapPathInvalidErr
		if errors.As(err, &dcErr) {
			err = ec(err, codes.E096.Ref(inv.Type.Spec))
		} else if errors.As(err, &pcErr) {
			err = ec(err, codes.E101.Ref(inv.Type.Spec))
		} else if errors.As(err, &piErr) {
			err = ec(err, codes.E099.Ref(inv.Type.Spec))
		}
		result.AddFatal(err)
	}
	// version names
	var versionNums ocfl.VNums = maps.Keys(inv.Versions)
	if err := versionNums.Valid(); err != nil {
		if errors.Is(err, ocfl.ErrVerEmpty) {
			err = ec(err, codes.E008.Ref(inv.Type.Spec))
		} else if errors.Is(err, ocfl.ErrVNumMissing) {
			err = ec(err, codes.E010.Ref(inv.Type.Spec))
		} else if errors.Is(err, ocfl.ErrVNumPadding) {
			err = ec(err, codes.E012.Ref(inv.Type.Spec))
		}
		result.AddFatal(err)
	}
	if versionNums.Head() != inv.Head {
		err := fmt.Errorf(`version head not most recent version: %s`, inv.Head)
		result.AddFatal(ec(err, codes.E040.Ref(inv.Type.Spec)))
	}
	// version state
	for vname, ver := range inv.Versions {
		err := ver.State.Valid()
		if err != nil {
			var dcErr *ocfl.MapDigestConflictErr
			var pcErr *ocfl.MapPathConflictErr
			var piErr *ocfl.MapPathInvalidErr
			if errors.As(err, &dcErr) {
				err = ec(err, codes.E050.Ref(inv.Type.Spec))
			} else if errors.As(err, &pcErr) {
				err = ec(err, codes.E095.Ref(inv.Type.Spec))
			} else if errors.As(err, &piErr) {
				err = ec(err, codes.E052.Ref(inv.Type.Spec))
			}
			result.AddFatal(err)
		}
		// check that each state digest appears in manifest
		for _, digest := range ver.State.Digests() {
			if len(inv.Manifest[digest]) == 0 {
				err := fmt.Errorf("digest in %s state not in manifest: %s", vname, digest)
				result.AddFatal(ec(err, codes.E050.Ref(inv.Type.Spec)))
			}
		}
		// version message
		if ver.Message == "" {
			err := fmt.Errorf("version %s missing recommended field: 'message'", vname)
			result.AddWarn(ec(err, codes.W007.Ref(inv.Type.Spec)))
		}
		if ver.User != nil {
			if ver.User.Name == "" {
				err := fmt.Errorf("version %s user missing required field: 'name'", vname)
				result.AddFatal(ec(err, codes.E054.Ref(inv.Type.Spec)))
			}
			if ver.User.Address == "" {
				err := fmt.Errorf("version %s user missing recommended field: 'address'", vname)
				result.AddWarn(ec(err, codes.W008.Ref(inv.Type.Spec)))
			}
			if u, err := url.ParseRequestURI(ver.User.Address); err != nil || u.Scheme == "" {
				err := fmt.Errorf("version %s user address is not a URI", vname)
				result.AddWarn(ec(err, codes.W009.Ref(inv.Type.Spec)))
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
			result.AddFatal(ec(err, codes.E107.Ref(inv.Type.Spec)))
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
				err = ec(err, codes.E097.Ref(inv.Type.Spec))
			} else if errors.As(err, &piErr) {
				err = ec(err, codes.E099.Ref(inv.Type.Spec))
			} else if errors.As(err, &pcErr) {
				err = ec(err, codes.E101.Ref(inv.Type.Spec))
			}
			result.AddFatal(err)
		}
	}
	return result
}

// ValidateInventory fully validates an inventory at path name in fsys.
func ValidateInventory(ctx context.Context, fsys ocfl.FS, name string, vops ...ValidationOption) (*Inventory, *validation.Result) {
	opts, invResult := validationSetup(vops)
	lgr := opts.Logger
	ocflV := opts.FallbackOCFL
	f, err := fsys.OpenFile(ctx, name)
	if err != nil {
		return nil, invResult.LogFatal(lgr, ec(err, codes.E063.Ref(ocflV)))
	}
	defer f.Close()
	invOpts := []ValidationOption{
		copyValidationOptions(opts),
		appendResult(invResult),
	}
	inv, _ := ValidateInventoryReader(ctx, f, invOpts...)
	if invResult.Err() != nil {
		return nil, invResult
	}
	ocflV = inv.Type.Spec
	side := name + "." + inv.DigestAlgorithm
	expSum, err := readInventorySidecar(ctx, fsys, side)
	if err != nil {
		if errors.Is(err, ErrInvSidecarContents) {
			return nil, invResult.LogFatal(lgr, ec(err, codes.E061.Ref(ocflV)))
		}
		return nil, invResult.LogFatal(lgr, ec(err, codes.E058.Ref(ocflV)))
	}
	if !strings.EqualFold(inv.digest, expSum) {
		shortSum := inv.digest[:6]
		shortExp := expSum[:6]
		err := fmt.Errorf("inventory's checksum (%s) doen't match expected value in sidecar (%s): %s", shortSum, shortExp, name)
		invResult.LogFatal(lgr, ec(err, codes.E060.Ref(ocflV)))
		return nil, invResult
	}
	return inv, invResult
}

func ValidateInventoryReader(ctx context.Context, reader io.Reader, vops ...ValidationOption) (*Inventory, *validation.Result) {
	opts, result := validationSetup(vops)
	lgr := opts.Logger
	var decInv decodeInventory
	sum, err := readDigestInventory(ctx, reader, &decInv)
	if err != nil {
		var decErr *InvDecodeError
		if errors.As(err, &decErr) {
			if decErr.ocflV.Empty() {
				decErr.ocflV = opts.FallbackOCFL
			}
			return nil, result.LogFatal(lgr, err)
		} else if errors.Is(err, fs.ErrNotExist) {
			result.LogFatal(lgr, ec(err, codes.E063.Ref(opts.FallbackOCFL)))
			return nil, result
		}
		result.LogFatal(lgr, ec(err, codes.E034.Ref(opts.FallbackOCFL)))
		return nil, result
	}
	// validate inventory and merge/log results. Use Log here because
	// asValidInventory doesn't do logging
	inv, invResult := decInv.asValidInventory()
	invResult.LogAll(lgr)
	result.Merge(invResult)
	if err := result.Err(); err != nil {
		return nil, result
	}
	inv.digest = sum
	return inv, result
}

// readDigestInventory reads and decodes the contents of file into the value
// pointed to by inv; it also digests the contents of the reader using the
// digest algorithm alg, returning the digest string.
func readDigestInventory(ctx context.Context, reader io.Reader, inv *decodeInventory) (string, error) {
	byt, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(byt, inv); err != nil {
		return "", err
	}
	if inv.DigestAlgorithm == nil {
		return "", errors.New("missing digest algorithm")
	}
	digester := ocfl.NewDigester(*inv.DigestAlgorithm)
	if digester == nil {
		return "", fmt.Errorf("%w: %q", ocfl.ErrUnknownAlg, *inv.DigestAlgorithm)
	}
	if _, err := io.Copy(digester, bytes.NewReader(byt)); err != nil {
		return "", err
	}
	return digester.String(), nil
}
