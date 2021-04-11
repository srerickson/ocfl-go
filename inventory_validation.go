package ocfl

import (
	"errors"
	"fmt"
)

func (inv *Inventory) Validate() error {
	// one or more versions are present
	if len(inv.Versions) == 0 {
		return &ValidationErr{
			err:  fmt.Errorf(`inventory has no versions`),
			code: &ErrE008,
		}
	}
	// id is present
	if inv.ID == "" {
		return &ValidationErr{
			err:  fmt.Errorf(`inventory missing 'id' field`),
			code: &ErrE036,
		}
	}
	// type is present
	if inv.Type == "" {
		return &ValidationErr{
			err:  fmt.Errorf(`inventory missing 'type' field`),
			code: &ErrE036,
		}
	}
	if inv.DigestAlgorithm == "" {
		return &ValidationErr{
			err:  fmt.Errorf(`inventory missing 'digestAlgorithm' field`),
			code: &ErrE036,
		}
	}
	// check verions sequence
	if err := versionSeqValid(inv.VersionDirs()); err != nil {
		return err
	}
	// check Head
	if err := inv.validateHead(); err != nil {
		return err
	}
	// TODO check contentDir
	// manifest is present (can be empty)
	if inv.Manifest == nil {
		return &ValidationErr{
			err:  fmt.Errorf(`inventory missing 'manifest' field`),
			code: &ErrE041,
		}
	}
	// check manifest path format
	err := inv.Manifest.Valid()
	if err != nil {
		if errors.Is(err, errDuplicateDigest) {
			return &ValidationErr{err: err, code: &ErrE096}
		}
		if errors.Is(err, errPathConflict) {
			return &ValidationErr{err: err, code: &ErrE095}
		}
		if errors.Is(err, errPathFormat) {
			return &ValidationErr{err: err, code: &ErrE099}
		}
		return err
	}
	// check version state path format
	for _, v := range inv.Versions {
		err := v.State.Valid()
		if err != nil {
			if errors.Is(err, errDuplicateDigest) {
				return &ValidationErr{err: err, code: &ErrE050}
			}
			if errors.Is(err, errPathConflict) {
				return &ValidationErr{err: err, code: &ErrE095}
			}
			if errors.Is(err, errPathFormat) {
				return &ValidationErr{err: err, code: &ErrE099}
			}
			return err
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
			return &ValidationErr{
				err:  fmt.Errorf("digest not used in state: %s", digest),
				code: &ErrE050,
			}
		}
	}
	// check version state path format
	for _, fixity := range inv.Fixity {
		err := fixity.Valid()
		if err != nil {
			if errors.Is(err, errDuplicateDigest) {
				return &ValidationErr{err: err, code: &ErrE097}
			}
			if errors.Is(err, errPathConflict) {
				return &ValidationErr{err: err, code: &ErrE095}
			}
			if errors.Is(err, errPathFormat) {
				return &ValidationErr{err: err, code: &ErrE099}
			}
			return err
		}
	}
	return nil
}

func (inv *Inventory) validateHead() error {
	v, _, err := versionParse(inv.Head)
	if err != nil {
		return &ValidationErr{
			err:  fmt.Errorf(`inventory 'head' not valid: %s`, inv.Head),
			code: &ErrE040,
		}
	}
	if _, ok := inv.Versions[inv.Head]; !ok {
		return &ValidationErr{
			err:  fmt.Errorf(`inventory 'head' value does not correspond to a version: %s`, inv.Head),
			code: &ErrE040,
		}
	}
	if v != len(inv.Versions) {
		return &ValidationErr{
			err:  fmt.Errorf(`inventory 'head' is not the last version %s`, inv.Head),
			code: &ErrE040,
		}
	}
	return nil
}
