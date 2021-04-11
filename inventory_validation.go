package ocfl

import (
	"errors"
	"fmt"
)

func (inv *Inventory) Validate() error {
	// one or more versions are present
	if len(inv.Versions) == 0 {
		return fmt.Errorf(`inventory missing 'versions' field: %w`, &ErrE008)
	}
	// id is present
	if inv.ID == "" {
		return fmt.Errorf(`inventory missing 'id' field: %w`, &ErrE036)
	}
	// type is present
	if inv.Type == "" {
		return fmt.Errorf(`inventory missing 'type' field: %w`, &ErrE036)
	}
	if inv.DigestAlgorithm == "" {
		return fmt.Errorf(`inventory missing 'digestAlgorithm' field: %w`, &ErrE036)
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
		return fmt.Errorf(`inventory missing 'manifest' field: %w`, &ErrE041)
	}
	// check manifest path format
	err := inv.Manifest.Valid()
	if err != nil {
		if errors.Is(err, errDuplicateDigest) {
			return &ErrE096
		}
		if errors.Is(err, errPathConflict) {
			return &ErrE095
		}
		if errors.Is(err, errPathFormat) {
			return &ErrE099
		}
		return err
	}
	// check version state path format
	for _, v := range inv.Versions {
		err := v.State.Valid()
		if err != nil {
			if errors.Is(err, errDuplicateDigest) {
				return &ErrE050
			}
			if errors.Is(err, errPathConflict) {
				return &ErrE095
			}
			if errors.Is(err, errPathFormat) {
				return &ErrE099
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
			return fmt.Errorf("digest not used in state: %s, %w", digest, &ErrE050)
		}
	}
	// check version state path format
	for _, fixity := range inv.Fixity {
		err := fixity.Valid()
		if err != nil {
			if errors.Is(err, errDuplicateDigest) {
				return &ErrE097
			}
			if errors.Is(err, errPathConflict) {
				return &ErrE095
			}
			if errors.Is(err, errPathFormat) {
				return &ErrE099
			}
			return err
		}
	}
	return nil
}

func (inv *Inventory) validateHead() error {
	v, _, err := versionParse(inv.Head)
	if err != nil {
		return fmt.Errorf(`inventory 'head' not valid: %w`, &ErrE040)
	}
	if _, ok := inv.Versions[inv.Head]; !ok {
		return fmt.Errorf(`inventory 'head' value does not correspond to a version: %w`, &ErrE040)
	}
	if v != len(inv.Versions) {
		return fmt.Errorf(`inventory 'head' is not the last version: %w`, &ErrE040)
	}
	return nil
}
