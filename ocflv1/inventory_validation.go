package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"strings"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/ocflv1/codes"
	"github.com/srerickson/ocfl/spec"
	"github.com/srerickson/ocfl/validation"
)

// Validate validates the inventory. It only checks the inventory's structure
// and internal consistency. The inventory is valid if the returned validation
// result includes no fatal errors (it may include warning errors).
func (inv *Inventory) Validate() *validation.Result {
	errs := &validation.Result{}
	if inv.ID == "" {
		err := errors.New("missing required field: 'id'")
		errs.AddFatal(ec(err, codes.E036.Ref(inv.Type.Num)))
	}
	if inv.Head.Empty() {
		err := errors.New("missing required field: 'head'")
		errs.AddFatal(ec(err, codes.E036.Ref(inv.Type.Num)))
	}
	if inv.ContentDirectory == "" {
		inv.ContentDirectory = contentDir
	}
	// don't continue if there are fatal errors
	if err := errs.Err(); err != nil {
		return errs
	}
	if u, err := url.ParseRequestURI(inv.ID); err != nil || u.Scheme == "" {
		err := fmt.Errorf(`object ID is not a URI: %s`, inv.ID)
		errs.AddWarn(ec(err, codes.W005.Ref(inv.Type.Num)))
	}
	if inv.DigestAlgorithm != digest.SHA512 {
		if inv.DigestAlgorithm != digest.SHA256 {
			err := fmt.Errorf(`digestAlgorithm is not %s or %s`, digest.SHA512, digest.SHA256)
			errs.AddFatal(ec(err, codes.E025.Ref(inv.Type.Num)))
		} else {
			err := fmt.Errorf(`digestAlgorithm is not %s`, digest.SHA512)
			errs.AddWarn(ec(err, codes.W004.Ref(inv.Type.Num)))
		}
	}
	if err := inv.Head.Valid(); err != nil {
		// this shouldn't ever trigger since the invalid condition is caught during unmarshal.
		err = fmt.Errorf("head is invalid: %w", err)
		errs.AddFatal(ec(err, codes.E011.Ref(inv.Type.Num)))
	}
	if strings.Contains(inv.ContentDirectory, "/") {
		err := errors.New("contentDirectory contains '/'")
		errs.AddFatal(ec(err, codes.E017.Ref(inv.Type.Num)))
	}
	if inv.ContentDirectory == "." || inv.ContentDirectory == ".." {
		err := errors.New("contentDirectory is '.' or '..'")
		errs.AddFatal(ec(err, codes.E017.Ref(inv.Type.Num)))
	}
	if err := inv.Manifest.Valid(); err != nil {
		var dcErr *digest.DigestConflictErr
		var bpErr *digest.BasePathErr
		var pcErr *digest.PathConflictErr
		var piErr *digest.PathInvalidErr
		if errors.As(err, &dcErr) {
			err = ec(err, codes.E096.Ref(inv.Type.Num))
		} else if errors.As(err, &bpErr) {
			err = ec(err, codes.E095.Ref(inv.Type.Num))
		} else if errors.As(err, &pcErr) {
			err = ec(err, codes.E101.Ref(inv.Type.Num))
		} else if errors.As(err, &piErr) {
			err = ec(err, codes.E099.Ref(inv.Type.Num))
		}
		errs.AddFatal(err)
	}

	// version names
	var versionNums ocfl.VNumSeq = make([]ocfl.VNum, 0, len(inv.Versions))
	for n := range inv.Versions {
		versionNums = append(versionNums, n)
	}
	if err := versionNums.Valid(); err != nil {
		if errors.Is(err, ocfl.ErrVerEmpty) {
			err = ec(err, codes.E008.Ref(inv.Type.Num))
		} else if errors.Is(err, ocfl.ErrVNumMissing) {
			err = ec(err, codes.E010.Ref(inv.Type.Num))
		} else if errors.Is(err, ocfl.ErrVNumPadding) {
			err = ec(err, codes.E012.Ref(inv.Type.Num))
		}
		errs.AddFatal(err)
	}
	if versionNums.Head() != inv.Head {
		err := fmt.Errorf(`version head not most recent version: %s`, inv.Head)
		errs.AddFatal(ec(err, codes.E040.Ref(inv.Type.Num)))
	}

	// version state
	for vname, ver := range inv.Versions {
		err := ver.State.Valid()
		if err != nil {
			var dcErr *digest.DigestConflictErr
			var bpErr *digest.BasePathErr
			var pcErr *digest.PathConflictErr
			var piErr *digest.PathInvalidErr
			if errors.As(err, &dcErr) {
				err = ec(err, codes.E050.Ref(inv.Type.Num))
			} else if errors.As(err, &bpErr) {
				err = ec(err, codes.E095.Ref(inv.Type.Num))
			} else if errors.As(err, &pcErr) {
				err = ec(err, codes.E095.Ref(inv.Type.Num))
			} else if errors.As(err, &piErr) {
				err = ec(err, codes.E052.Ref(inv.Type.Num))
			}
			errs.AddFatal(err)
		}
		// check that each state digest appears in manifest
		for digest := range ver.State.AllDigests() {
			if !inv.Manifest.DigestExists(digest) {
				err := fmt.Errorf("digest in %s state not in manifest: %s", vname, digest)
				errs.AddFatal(ec(err, codes.E050.Ref(inv.Type.Num)))
			}
		}
		// version message
		if ver.Message == "" {
			err := fmt.Errorf("version %s missing recommended field: 'message'", vname)
			errs.AddWarn(ec(err, codes.W007.Ref(inv.Type.Num)))
		}
		if ver.User != nil {
			if ver.User.Address == "" {
				err := fmt.Errorf("version %s user missing recommended field: 'address'", vname)
				errs.AddWarn(ec(err, codes.W008.Ref(inv.Type.Num)))
			}
			if u, err := url.ParseRequestURI(ver.User.Address); err != nil || u.Scheme == "" {
				err := fmt.Errorf("version %s user address is not a URI", vname)
				errs.AddWarn(ec(err, codes.W009.Ref(inv.Type.Num)))
			}
		}
	}
	// check that each manifest entry is used in at least one state
	for digest := range inv.Manifest.AllDigests() {
		var found bool
		for _, version := range inv.Versions {
			if version.State == nil {
				continue
			}
			if version.State.DigestExists(digest) {
				found = true
				break
			}
		}
		if !found {
			err := fmt.Errorf("digest in manifest not used in version state: %s", digest)
			errs.AddFatal(ec(err, codes.E107.Ref(inv.Type.Num)))
		}
	}
	//fixity
	for _, fixity := range inv.Fixity {
		err := fixity.Valid()
		if err != nil {
			var dcErr *digest.DigestConflictErr
			var piErr *digest.PathInvalidErr
			var pcErr *digest.PathConflictErr
			if errors.As(err, &dcErr) {
				err = ec(err, codes.E097.Ref(inv.Type.Num))
			} else if errors.As(err, &piErr) {
				err = ec(err, codes.E099.Ref(inv.Type.Num))
			} else if errors.As(err, &pcErr) {
				err = ec(err, codes.E101.Ref(inv.Type.Num))
			}
			errs.AddFatal(err)
		}
	}
	return errs

}

// ValidateInventoryConf
type ValidateInventoryConf struct {
	validation.Log
	// fs.FS for opening inventory file.
	FS fs.FS
	// path of inventory file relative to FS. This options supercedes the
	// Reader option
	Name string
	// Reader for inventory content
	Reader io.Reader
	// algorithm for sidecar file
	DigestAlgorithm digest.Alg
	// skip sidecar verification
	SkipSidecar bool
	// if the inventory's OCFL version cannot be determined, validation errors
	// will reference this version of the OCFL spec.
	FallbackOCFL spec.Num
}

// ValidateInventory full validates an inventory based on the configuration.
func ValidateInventory(ctx context.Context, cfg *ValidateInventoryConf) (*Inventory, error) {
	var c ValidateInventoryConf
	if cfg != nil {
		c = *cfg
	}
	if c.FallbackOCFL.Empty() {
		c.FallbackOCFL = ocflv1_0
	}
	if c.FS != nil && c.Name != "" {
		f, err := c.FS.Open(c.Name)
		if err != nil {
			return nil, c.AddFatal(ec(err, codes.E063.Ref(c.FallbackOCFL)))
		}
		defer f.Close()
		c.Reader = f.(io.Reader)
	}
	if c.Reader == nil {
		return nil, errors.New("validating inventory: nothing to read")
	}
	decInv, err := decodeInv(ctx, &c)
	if err != nil {
		return nil, err
	}
	inv, errs := decInv.asValidInventory()
	c.Log.AddResult(errs)
	if err := c.Log.Err(); err != nil {
		return nil, err
	}
	return inv, nil
}

func decodeInv(ctx context.Context, conf *ValidateInventoryConf) (*decodeInventory, error) {
	var inv decodeInventory
	sum, err := ReadDigestInventory(ctx, conf.Reader, &inv, conf.DigestAlgorithm)
	if err != nil {
		var decErr *InvDecodeError
		if errors.As(err, &decErr) {
			// check if the decode error includes an OCFL reference already
			if ref := decErr.OCFLRef(); ref != nil {
				return nil, conf.AddFatal(err)
			}
			// otherwise set fallback OCFL version
			decErr.ocflV = conf.FallbackOCFL
			return nil, conf.AddFatal(decErr)
		} else if errors.Is(err, ErrInventoryOpen) {
			return nil, conf.AddFatal(ec(err, codes.E063.Ref(conf.FallbackOCFL)))
		}
		return nil, conf.AddFatal(ec(err, codes.E034.Ref(conf.FallbackOCFL)))
	}
	if conf.DigestAlgorithm.ID() == "" {
		conf.DigestAlgorithm = *inv.DigestAlgorithm
	}
	if !conf.SkipSidecar && conf.FS != nil {
		// confirm sidecar
		side := conf.Name + "." + conf.DigestAlgorithm.ID()
		sidecarReader, err := conf.FS.Open(side)
		if err != nil {
			return nil, conf.AddFatal(ec(err, codes.E058.Ref(conf.FallbackOCFL)))
		}
		defer sidecarReader.Close()
		expSum, err := ReadInventorySidecar(ctx, sidecarReader)
		if err != nil {
			return nil, conf.AddFatal(ec(err, codes.E061.Ref(conf.FallbackOCFL)))
		}
		if !strings.EqualFold(sum, expSum) {
			shortSum := sum[:6]
			shortExp := expSum[:6]
			err := fmt.Errorf("inventory's checksum (%s) doen't match expected value in sidecar (%s): %s", shortSum, shortExp, conf.Name)
			return nil, conf.AddFatal(ec(err, codes.E060.Ref(inv.ocflV)))
		}
	}
	inv.digest = sum
	return &inv, nil
}
