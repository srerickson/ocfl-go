package internal

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/qri-io/jsonschema"
	"github.com/srerickson/ocfl/internal/schema"
)

var invJsonSchema *jsonschema.Schema

func init() {
	// required for the schema to work
	jsonschema.RegisterKeyword("definitions", jsonschema.NewDefs)
	jsonschema.LoadDraft2019_09()
	invJsonSchema = jsonschema.Must(string(schema.InventorySchema))
}

func validateInventoryBytes(inv []byte) validationResult {
	result := validationResult{}
	errs, err := invJsonSchema.ValidateBytes(context.Background(), inv)
	if err != nil {
		result.AddFatal(err, nil)
		return result
	}
	for _, e := range errs {
		// FIXME this string matching business is crude
		if strings.Contains(e.Message, `"id"`) {
			result.AddFatal(e, &ErrE036)
		} else if strings.Contains(e.Message, `"head"`) {
			result.AddFatal(e, &ErrE036)
		} else if strings.Contains(e.Message, `"type"`) {
			result.AddFatal(e, &ErrE036)
		} else if strings.Contains(e.Message, `"digestAlgorithm"`) {
			result.AddFatal(e, &ErrE036)
		} else if strings.Contains(e.Message, `"versions"`) {
			result.AddFatal(e, &ErrE041)
		} else if strings.Contains(e.Message, `"manifest"`) {
			result.AddFatal(e, &ErrE041)
		} else if strings.Contains(e.Message, `array items must be unique`) {
			result.AddFatal(e, &ErrE095)
		} else {
			result.AddFatal(e, &ErrE032)
		}
	}
	return result
}

func (inv *Inventory) Validate() error {
	// one or more versions are present
	// if len(inv.Versions) == 0 {
	// 	return &validationErr{
	// 		err:  fmt.Errorf(`inventory has no versions`),
	// 		code: &ErrE008,
	// 	}
	// }
	// ID
	// E037 - '[id] must be unique in the local context, and should be a URI [RFC3986].'
	// W005 - 'The OCFL Object Inventory id SHOULD be a URI.'
	// if inv.ID == "" {
	// 	return &validationErr{
	// 		err:  fmt.Errorf(`inventory missing 'id' field`),
	// 		code: &ErrE036,
	// 	}
	// }
	// Type
	// E038 - must be the URI of the inventory section of the specification version matching the object conformance declaration.
	// if inv.Type == "" {
	// 	return &validationErr{
	// 		err:  fmt.Errorf(`inventory missing 'type' field`),
	// 		code: &ErrE036,
	// 	}
	// }
	// Digest Algorithm
	// E025 - OCFL Objects must use either sha512 or sha256, and should use sha512
	// if inv.DigestAlgorithm == "" {
	// 	return &validationErr{
	// 		err:  fmt.Errorf(`inventory missing 'digestAlgorithm' field`),
	// 		code: &ErrE036,
	// 	}
	// }
	// Versions
	// E043 - 'An OCFL Object Inventory must include a block for storing versions.'
	// E046 - 'The keys of [the versions object] must correspond to the names of the version directories used.'
	if err := versionSeqValid(inv.VersionDirs()); err != nil {
		return err
	}
	// Head
	// E040 - [head] must be the version directory name with the highest version number.'
	if err := inv.validateHead(); err != nil {
		return err
	}
	// ContentDir
	// E017/18 - 'The contentDirectory value MUST NOT contain the forward slash (/) path
	// 	separator and must not be either one or two periods (. or ..).'
	// E019/20 - 'If the key contentDirectory is set, it MUST be set in the first version of the object
	//	and must not change between versions of the same object.'
	// E021 - 'If the key contentDirectory is not present in the inventory file then
	//  the name of the designated content sub-directory must be content.'
	// Manifest

	if inv.Manifest == nil {
		return &validationErr{
			err:  fmt.Errorf(`inventory missing 'manifest' field`),
			code: &ErrE041,
		}
	}
	// check manifest path format
	err := inv.Manifest.Valid()
	if err != nil {
		var dcErr *DigestConflictErr
		if errors.As(err, &dcErr) {
			return &validationErr{err: err, code: &ErrE096}
		}
		var pcErr *PathConflictErr
		if errors.As(err, &pcErr) {
			return &validationErr{err: err, code: &ErrE095}
		}
		var piErr *PathInvalidErr
		if errors.As(err, &piErr) {
			return &validationErr{err: err, code: &ErrE099}
		}
		return err
	}
	// Version State
	// E050 - 'The keys of [the "state" JSON object] are digest values, each of which must
	//	correspond to an entry in the manifest of the inventory.'
	// E095 - 'Within a version, logical paths must be unique and non-conflicting, so the
	//	logical path for a file cannot appear as the initial part of another logical path.'
	for _, v := range inv.Versions {
		err := v.State.Valid()
		if err != nil {
			var dcErr *DigestConflictErr
			if errors.As(err, &dcErr) {
				// FIXME - E050 seems wrong
				return &validationErr{err: err, code: &ErrE050}
			}
			var pcErr *PathConflictErr
			if errors.As(err, &pcErr) {
				return &validationErr{err: err, code: &ErrE095}
			}
			var piErr *PathInvalidErr
			if errors.As(err, &piErr) {
				return &validationErr{err: err, code: &ErrE099}
			}
			return err
		}
		// check that each state digest appears in manifest
		for digest := range v.State {
			if _, exists := inv.Manifest[digest]; !exists {
				return &validationErr{
					err:  fmt.Errorf("digest in % state not in manifest: %s", v, digest),
					code: &ErrE050,
				}
			}
		}
	}
	// check that each manifest entry is used in at least one state
	for digest := range inv.Manifest {
		var found bool
		for _, version := range inv.Versions {
			for d := range version.State {
				if digest == d {
					found = true
				}
			}
		}
		if !found {
			// This error code is used in the fixture
			// but doesn't makesense
			return &validationErr{
				err:  fmt.Errorf("digest not used in state: %s", digest),
				code: &ErrE050,
			}
		}
	}
	// check version state path format
	for _, fixity := range inv.Fixity {
		err := fixity.Valid()
		if err != nil {
			var dcErr *DigestConflictErr
			if errors.As(err, &dcErr) {
				return &validationErr{err: err, code: &ErrE097}
			}
			var pcErr *PathConflictErr
			if errors.As(err, &pcErr) {
				return &validationErr{err: err, code: &ErrE095}
			}
			var piErr *PathInvalidErr
			if errors.As(err, &piErr) {
				return &validationErr{err: err, code: &ErrE099}
			}
			return err
		}
	}
	return nil
}

func (inv *Inventory) validateHead() error {
	v, _, err := versionParse(inv.Head)
	if err != nil {
		return &validationErr{
			err:  fmt.Errorf(`inventory 'head' not valid: %s`, inv.Head),
			code: &ErrE040,
		}
	}
	if _, ok := inv.Versions[inv.Head]; !ok {
		return &validationErr{
			err:  fmt.Errorf(`inventory 'head' value does not correspond to a version: %s`, inv.Head),
			code: &ErrE040,
		}
	}
	if v != len(inv.Versions) {
		return &validationErr{
			err:  fmt.Errorf(`inventory 'head' is not the last version %s`, inv.Head),
			code: &ErrE040,
		}
	}
	return nil
}
