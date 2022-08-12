package mutate

// mutate is a WIP: store creation, committing, etc.

import (
	"errors"
	"fmt"
	"path"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/ocflv1"
	"github.com/srerickson/ocfl/spec"
)

// next version returns the next version of an inventory based on stage
func nextVersionInventory(prev *ocflv1.Inventory, stg *objStage) (*ocflv1.Inventory, error) {
	newInv := &ocflv1.Inventory{
		Versions: map[ocfl.VNum]*ocflv1.Version{},
		Manifest: digest.NewMap(),
	}
	if prev != nil {
		// matching object ID
		if prev.ID != stg.ObjectID() {
			return nil, fmt.Errorf("unexpected object ID: %s", stg.ObjectID())
		}
		// correct version
		expV, err := prev.Head.Next()
		if err != nil {
			return nil, fmt.Errorf("invalid next version: %w", err)
		}
		if expV != stg.VersionNum() {
			err := fmt.Errorf("stage version (%s) is not expected version (%s)",
				stg.VersionNum().String(), expV.String())
			return nil, err
		}
		// matching digest algorithm
		if stg.DigestAlgorithm() != prev.DigestAlgorithm {
			err := errors.New("stage and inventory have different digest algorithms")
			return nil, err
		}
		// copy version state and manifest
		for num, ver := range prev.Versions {
			newInv.Versions[num] = ver
		}
		newInv.Manifest = prev.Manifest
	} else {
		// new inventory
		if stg.VersionNum().Num() != 1 {
			err := fmt.Errorf("expected v1 stage, got: %s", stg.VersionNum().String())
			return nil, err
		}
	}
	newInv.ID = stg.ObjectID()
	newInv.Head = stg.VersionNum()
	newInv.Type = spec.Num{1, 0}.AsInventoryType() // FIXME
	newInv.DigestAlgorithm = stg.DigestAlgorithm()
	newInv.ContentDirectory = contentDir
	newInv.Versions[stg.VersionNum()] = &ocflv1.Version{
		State:   stg.Logical,
		Message: stg.Message,
		User: &ocflv1.User{
			Name:    stg.User.Name,
			Address: stg.User.Address,
		},
	}

	// manifest
	var err error
	newInv.Manifest, err = stg.BuildManifest(func(p string) string {
		return path.Join(newInv.Head.String(), newInv.ContentDirectory, p)
	})
	if err != nil {
		return nil, fmt.Errorf("generating new manifest: %w", err)
	}

	return newInv, nil
}
