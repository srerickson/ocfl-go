package ocflv1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/ocflv1/codes"
	"github.com/srerickson/ocfl/validation"
)

// decodeInventory is an internal type used exclusively for reading/decoding
// inventory files. The main difference between decodeInventory and Inventory
// is that a decodeInventory values are pointers.
type decodeInventory struct {
	ID               *string                `json:"id"`
	Type             *ocfl.InvType          `json:"type"`
	DigestAlgorithm  *string                `json:"digestAlgorithm"`
	Head             *ocfl.VNum             `json:"head"`
	ContentDirectory *string                `json:"contentDirectory,omitempty"`
	Manifest         *manifest              `json:"manifest"`
	Versions         map[ocfl.VNum]*version `json:"versions"`
	Fixity           fixity                 `json:"fixity,omitempty"`

	// private
	ocflV ocfl.Spec // OCFL version determined during UnmarshalJSON
	digest.Alg
	digest string
}

// validateNils checks that none of the inventory's required fields have nil
// values. An the returned Result will include a fatal error for each nil value
// encountered.
func (inv *decodeInventory) validateNils() *validation.Result {
	result := validation.NewResult(-1)
	if inv.ID == nil {
		err := errors.New("missing required field: 'id'")
		result.AddFatal(ec(err, codes.E036.Ref(inv.ocflV)))
	}
	if inv.DigestAlgorithm == nil {
		err := errors.New("missing required field: 'digestAlgorithm'")
		result.AddFatal(ec(err, codes.E036.Ref(inv.ocflV)))
	}
	if inv.Head == nil {
		err := errors.New("missing required field: 'head'")
		result.AddFatal(ec(err, codes.E036.Ref(inv.ocflV)))
	}
	if inv.Manifest == nil {
		err := fmt.Errorf("missing required field: 'manifest'")
		result.AddFatal(ec(err, codes.E041.Ref(inv.ocflV)))
	}
	if inv.Versions == nil {
		err := errors.New("missing required field: 'versions'")
		result.AddFatal(ec(err, codes.E041.Ref(inv.ocflV)))
	}
	for vname, ver := range inv.Versions {
		if ver == nil {
			err := fmt.Errorf("version %s missing value", vname)
			result.AddFatal(ec(err, codes.E048.Ref(inv.ocflV)))
			continue
		}
		if ver.Created == nil {
			err := fmt.Errorf("version %s missing required field: 'created'", vname)
			result.AddFatal(ec(err, codes.E048.Ref(inv.ocflV)))
		}
		if ver.State == nil {
			err := fmt.Errorf("version %s missing required field: 'state'", vname)
			result.AddFatal(ec(err, codes.E048.Ref(inv.ocflV)))
		}
		if ver.User != nil {
			if ver.User.Name == nil {
				err := fmt.Errorf("version %s user missing required field: 'name'", vname)
				result.AddFatal(ec(err, codes.E054.Ref(inv.ocflV)))
			}
		}
	}
	return result
}

func (inv decodeInventory) contentDirectory() string {
	if inv.ContentDirectory == nil {
		return contentDir
	}
	return *inv.ContentDirectory
}

// asInventory converts inv to an Inventory. If the inv cannot be converted due
// to nil values, the returned validation.Result includes fatal errors. Otherwise,
// the validation.Result is nil.
func (inv decodeInventory) asInventory() (*Inventory, *validation.Result) {
	result := inv.validateNils()
	if result.Err() != nil {
		return nil, result
	}
	newInv := &Inventory{
		ID:               *inv.ID,
		Type:             *inv.Type,
		Head:             *inv.Head,
		ContentDirectory: inv.contentDirectory(),
		DigestAlgorithm:  *inv.DigestAlgorithm,
		Manifest:         &inv.Manifest.Map,
		Fixity:           inv.Fixity,
		digest:           inv.digest,
	}
	newInv.Versions = make(map[ocfl.VNum]*Version, len(inv.Versions))
	for num, ver := range inv.Versions {
		newInv.Versions[num] = ver.Version()
	}
	return newInv, result
}

// asValidInventory converts inv to an Inventory and checks its validity. If the
// inv cannot be converted, or if the new Inventory is not valid, the returned
// validation.Result will include fatal errors. The result is always non-nil and
// has no associated Logger (errors have not been logged).
func (inv decodeInventory) asValidInventory() (*Inventory, *validation.Result) {
	newInv, result := inv.asInventory()
	if err := result.Err(); err != nil {
		return nil, result
	}
	result.Merge(newInv.Validate())
	if err := result.Err(); err != nil {
		return nil, result
	}
	return newInv, result
}

func (inv *decodeInventory) UnmarshalJSON(b []byte) error {
	// determine inventory type/version
	var justType struct {
		Type ocfl.InvType `json:"type"`
	}
	err := json.Unmarshal(b, &justType)
	if err != nil {
		return &InvDecodeError{
			error: err,
			Field: "type",
		}
	}
	ocflV := justType.Type.Spec
	if ocflV.Empty() {
		return &InvDecodeError{
			error: fmt.Errorf("can't determine inventory type/OCFL version"),
			Field: "type",
		}
	}
	type invAlias decodeInventory
	alias := (*invAlias)(inv)
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	err = dec.Decode(alias)
	if err != nil {
		var invErr *InvDecodeError
		if errors.As(err, &invErr) {
			invErr.ocflV = ocflV
			return err
		}
		var jsonErr *json.UnmarshalTypeError
		if errors.As(err, &jsonErr) {
			return &InvDecodeError{
				error: err,
				Field: jsonErr.Field,
				ocflV: ocflV,
			}
		}
		if errors.Is(err, ocfl.ErrVNumInvalid) {
			return &InvDecodeError{
				error: err,
				Field: "head",
				ocflV: ocflV,
			}
		}
		// Unknown Field Error
		if strings.HasPrefix(err.Error(), "json: unknown field") {
			return &InvDecodeError{
				error:   err,
				Unknown: true,
				Field:   strings.TrimPrefix(err.Error(), "json: unknown field "),
				ocflV:   ocflV,
			}
		}

		return &InvDecodeError{
			error: err,
			ocflV: ocflV,
		}
	}
	inv.ocflV = ocflV
	return nil
}

// manifest is an internal type that implements json.Unmarshaler
type manifest struct {
	digest.Map
}

func (m *manifest) UnmarshalJSON(b []byte) error {
	var dm digest.Map
	err := json.Unmarshal(b, &dm)
	if err != nil {
		return &InvDecodeError{Field: `manifest`, error: err}
	}
	m.Map = dm
	return nil
}

// fixity is an internal type that implements json.Unmarshaler
type fixity map[string]*digest.Map

func (f *fixity) UnmarshalJSON(b []byte) error {
	var newF map[string]*digest.Map
	err := json.Unmarshal(b, &newF)
	if err != nil {
		return &InvDecodeError{Field: `fixity`, error: err}
	}
	*f = newF
	return nil
}

// version is an internal type that implements json.Unmarshaler
type version struct {
	Created *time.Time  `json:"created"`
	State   *digest.Map `json:"state"`
	Message *string     `json:"message,omitempty"`
	User    *user       `json:"user,omitempty"`
}

func (v version) Version() *Version {
	newVer := &Version{
		Created: *v.Created,
		State:   v.State,
	}
	if v.Message != nil {
		newVer.Message = *v.Message
	}
	if v.User != nil {
		newVer.User = &User{Name: *v.User.Name}
		if v.User.Address != nil {
			newVer.User.Address = *v.User.Address
		}

	}
	return newVer
}

func (v *version) UnmarshalJSON(b []byte) error {
	type VersionAlias version
	alias := &VersionAlias{}
	err := json.Unmarshal(b, alias)
	if err != nil {
		return &InvDecodeError{Field: `version`, error: err}
	}
	*v = version(*alias)
	return nil
}

// user is an internal type that implements json.Unmarshaler
type user struct {
	Name    *string `json:"name,omitempty"`
	Address *string `json:"address,omitempty"`
}

// InvDecodeError wraps errors generated during inventory unmarshaling.
// It implements the validation.ErrorCode interface so instances can
// return spec error codes.
type InvDecodeError struct {
	error
	Field   string
	Unknown bool
	ocflV   ocfl.Spec
}

// InvDecodeError implements validation.ErrorCode
var _ validation.ErrorCode = &InvDecodeError{}

func (invErr *InvDecodeError) Error() string {
	if invErr.Field != "" {
		return fmt.Sprintf("error in inventory '%s': %s", invErr.Field, invErr.error.Error())
	}
	return fmt.Sprintf("error in inventory: %s", invErr.error.Error())
}

func (invErr *InvDecodeError) Unwrap() error {
	return invErr.error
}

func (invErr *InvDecodeError) OCFLRef() *validation.Ref {
	switch invErr.Field {
	case "head":
		return codes.E104.Ref(invErr.ocflV)
	case "type":
		return codes.E038.Ref(invErr.ocflV)
	case "version":
		switch err := invErr.error.(type) {
		case *time.ParseError:
			// error parsing version.created
			return codes.E049.Ref(invErr.ocflV)
		case *json.UnmarshalTypeError:
			if err.Field == `versions.message` {
				return codes.E094.Ref(invErr.ocflV)
			}
		}
	}
	// Unknown Field Error
	if strings.HasPrefix(invErr.error.Error(), "json: unknown field") {
		return codes.E102.Ref(invErr.ocflV)
	}
	return nil
}
