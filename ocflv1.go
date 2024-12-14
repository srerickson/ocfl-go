package ocfl

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/ocflv1/codes"
)

var OCFLv1_0 = ocflV1(Spec1_0)
var OCFLv1_1 = ocflV1(Spec1_1)

// implementation of ocfl v1.x
type ocflV1 Spec

func (imp ocflV1) Spec() Spec {
	return Spec(imp)
}

func (imp ocflV1) ValidateInventory(inv *RawInventory) *Validation {
	v := &Validation{}
	if inv.Type.Empty() {
		err := errors.New("missing required field: 'type'")
		v.AddFatal(err)
	}
	ocflV := string(imp)
	if inv.Type.Spec != imp.Spec() {
		err := fmt.Errorf("inventory declares v%s, not v%s", ocflV, ocflV)
		v.AddFatal(err)
	}
	if inv.ID == "" {
		err := errors.New("missing required field: 'id'")
		v.AddFatal(verr(err, codes.E036(ocflV)))
	}
	if inv.Head.IsZero() {
		err := errors.New("missing required field: 'head'")
		v.AddFatal(verr(err, codes.E036(ocflV)))
	}
	if inv.Manifest == nil {
		err := errors.New("missing required field 'manifest'")
		v.AddFatal(verr(err, codes.E041(ocflV)))
	}
	if inv.Versions == nil {
		err := errors.New("missing required field: 'versions'")
		v.AddFatal(verr(err, codes.E041(ocflV)))
	}
	if u, err := url.ParseRequestURI(inv.ID); err != nil || u.Scheme == "" {
		err := fmt.Errorf(`object ID is not a URI: %q`, inv.ID)
		v.AddWarn(verr(err, codes.W005(ocflV)))
	}
	switch inv.DigestAlgorithm {
	case digest.SHA512.ID():
		break
	case digest.SHA256.ID():
		err := fmt.Errorf(`'digestAlgorithm' is %q`, digest.SHA256.ID())
		v.AddWarn(verr(err, codes.W004(ocflV)))
	default:
		err := fmt.Errorf(`'digestAlgorithm' is not %q or %q`, digest.SHA512.ID(), digest.SHA256.ID())
		v.AddFatal(verr(err, codes.E025(ocflV)))
	}
	if err := inv.Head.Valid(); err != nil {
		err = fmt.Errorf("head is invalid: %w", err)
		v.AddFatal(verr(err, codes.E011(ocflV)))
	}
	if strings.Contains(inv.ContentDirectory, "/") {
		err := errors.New("contentDirectory contains '/'")
		v.AddFatal(verr(err, codes.E017(ocflV)))
	}
	if inv.ContentDirectory == "." || inv.ContentDirectory == ".." {
		err := errors.New("contentDirectory is '.' or '..'")
		v.AddFatal(verr(err, codes.E017(ocflV)))
	}
	if inv.Manifest != nil {
		err := inv.Manifest.Valid()
		if err != nil {
			var dcErr *MapDigestConflictErr
			var pcErr *MapPathConflictErr
			var piErr *MapPathInvalidErr
			if errors.As(err, &dcErr) {
				err = verr(err, codes.E096(ocflV))
			} else if errors.As(err, &pcErr) {
				err = verr(err, codes.E101(ocflV))
			} else if errors.As(err, &piErr) {
				err = verr(err, codes.E099(ocflV))
			}
			v.AddFatal(err)
		}
		// check that each manifest entry is used in at least one state
		for _, digest := range inv.Manifest.Digests() {
			var found bool
			for _, version := range inv.Versions {
				if version == nil {
					continue
				}
				if len(version.State[digest]) > 0 {
					found = true
					break
				}
			}
			if !found {
				err := fmt.Errorf("digest in manifest not used in version state: %s", digest)
				v.AddFatal(verr(err, codes.E107(ocflV)))
			}
		}
	}
	// version names
	var versionNums VNums = inv.VNums()
	if err := versionNums.Valid(); err != nil {
		if errors.Is(err, ErrVerEmpty) {
			err = verr(err, codes.E008(ocflV))
		} else if errors.Is(err, ErrVNumMissing) {
			err = verr(err, codes.E010(ocflV))
		} else if errors.Is(err, ErrVNumPadding) {
			err = verr(err, codes.E012(ocflV))
		}
		v.AddFatal(err)
	}
	if versionNums.Head() != inv.Head {
		err := fmt.Errorf(`version head not most recent version: %s`, inv.Head)
		v.AddFatal(verr(err, codes.E040(ocflV)))
	}
	// version state
	for vname, ver := range inv.Versions {
		if ver == nil {
			err := fmt.Errorf(`missing required version block for %q`, vname)
			v.AddFatal(verr(err, codes.E048(ocflV)))
			continue
		}
		if ver.Created.IsZero() {
			err := fmt.Errorf(`version %s missing required field: 'created'`, vname)
			v.AddFatal(verr(err, codes.E048(ocflV)))
		}
		if ver.Message == "" {
			err := fmt.Errorf("version %s missing recommended field: 'message'", vname)
			v.AddWarn(verr(err, codes.W007(ocflV)))
		}
		if ver.User == nil {
			err := fmt.Errorf("version %s missing recommended field: 'user'", vname)
			v.AddWarn(verr(err, codes.W007(ocflV)))
		}
		if ver.User != nil {
			if ver.User.Name == "" {
				err := fmt.Errorf("version %s user missing required field: 'name'", vname)
				v.AddFatal(verr(err, codes.E054(ocflV)))
			}
			if ver.User.Address == "" {
				err := fmt.Errorf("version %s user missing recommended field: 'address'", vname)
				v.AddWarn(verr(err, codes.W008(ocflV)))
			}
			if u, err := url.ParseRequestURI(ver.User.Address); err != nil || u.Scheme == "" {
				err := fmt.Errorf("version %s user address is not a URI", vname)
				v.AddWarn(verr(err, codes.W009(ocflV)))
			}
		}
		if ver.State == nil {
			err := fmt.Errorf(`version %s missing required field: 'state'`, vname)
			v.AddFatal(verr(err, codes.E048(ocflV)))
			continue
		}
		err := ver.State.Valid()
		if err != nil {
			var dcErr *MapDigestConflictErr
			var pcErr *MapPathConflictErr
			var piErr *MapPathInvalidErr
			if errors.As(err, &dcErr) {
				err = verr(err, codes.E050(ocflV))
			} else if errors.As(err, &pcErr) {
				err = verr(err, codes.E095(ocflV))
			} else if errors.As(err, &piErr) {
				err = verr(err, codes.E052(ocflV))
			}
			v.AddFatal(err)
		}
		// check that each state digest appears in manifest
		for _, digest := range ver.State.Digests() {
			if len(inv.Manifest[digest]) == 0 {
				err := fmt.Errorf("digest in %s state not in manifest: %s", vname, digest)
				v.AddFatal(verr(err, codes.E050(ocflV)))
			}
		}
	}
	//fixity
	for _, fixity := range inv.Fixity {
		err := fixity.Valid()
		if err != nil {
			var dcErr *MapDigestConflictErr
			var piErr *MapPathInvalidErr
			var pcErr *MapPathConflictErr
			if errors.As(err, &dcErr) {
				err = verr(err, codes.E097(ocflV))
			} else if errors.As(err, &piErr) {
				err = verr(err, codes.E099(ocflV))
			} else if errors.As(err, &pcErr) {
				err = verr(err, codes.E101(ocflV))
			}
			v.AddFatal(err)
		}
	}
	return v
}

func (imp *ocflV1) NewReadInventory(raw []byte) (*RawInventoryVersion, error) {
	return nil, errors.New("not implemented")
}

func (imp *ocflV1) NewReadObject(fsys FS, path string, inv ReadInventory) ReadObject {
	return nil
}
func (imp *ocflV1) Commit(ctx context.Context, obj ReadObject, commit *Commit) (ReadObject, error) {
	return nil, errors.New("not implemented")
}
func (imp *ocflV1) ValidateObjectRoot(ctx context.Context, fs FS, dir string, state *ObjectState, vldr *ObjectValidation) (ReadObject, error) {
	return nil, errors.New("not implemented")

}
func (imp *ocflV1) ValidateObjectVersion(ctx context.Context, obj ReadObject, vnum VNum, versionInv ReadInventory, prevInv ReadInventory, vldr *ObjectValidation) error {
	return errors.New("not implemented")

}
func (imp *ocflV1) ValidateObjectContent(ctx context.Context, obj ReadObject, vldr *ObjectValidation) error {
	return errors.New("not implemented")

}
