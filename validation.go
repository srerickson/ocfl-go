// Copyright 2019 Seth R. Erickson
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ocfl

import (
	"context"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

// Validator handles state for OCFL Object validation
type Validator struct {
	root      string
	inventory *Inventory
	Cancel    context.CancelFunc
	errChan   chan error
}

// NewValidator returns a new validator
func NewValidator(ctx context.Context, dir string) (Validator, context.Context) {
	v := Validator{
		root:    dir,
		errChan: make(chan error),
	}
	ctx, v.Cancel = context.WithCancel(ctx)
	return v, ctx
}

// handleErr sends the error over the channel if the
// context is still active, otherwise it returns the error
func (v *Validator) handleErr(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return err
	default:
		v.errChan <- err
	}
	return nil
}

func done(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
	}
	return false
}

// ValidateObject validates the object at path
func ValidateObject(path string) error {
	ctx := context.Background()
	v, _ := NewValidator(ctx, path)
	for vErr := range v.ValidateObject(ctx, path) {
		// cancel remaining validation on first error
		v.Cancel()
		return vErr
	}
	return nil
}

// ValidateObject validates OCFL object located at path
func (v *Validator) ValidateObject(ctx context.Context, path string) chan error {
	go func() {
		defer close(v.errChan)
		// Load Object
		obj, err := GetObject(path)
		if err != nil {
			v.handleErr(ctx, err)
			return
		}
		v.validateInventory(ctx, &(obj.inventory))
		v.root = obj.Path
		v.inventory = &obj.inventory
		alg := v.inventory.DigestAlgorithm
		// Validate Each Version Directory
		files, ioErr := ioutil.ReadDir(path)
		if ioErr != nil {
			v.handleErr(ctx, NewErr(ReadErr, ioErr))
			return
		}
		for _, f := range files {
			// check if context is canceled
			if done(ctx) {
				break
			}
			if !f.IsDir() {
				continue
			}
			if style := versionFormat(f.Name()); style != `` {
				v.validateVersionDir(ctx, f.Name())
			}
		}
		// Manifest Checksum
		v.validateContentMap(ctx, v.inventory.Manifest, alg)

		// Fixity Checksum
		// for alg, manifest := range v.inventory.Fixity {
		// 	if err := manifest.Validate(v.root, alg); err != nil {
		// 		retErr = err
		// 	}
		// }

	}()
	return v.errChan
}

func (v *Validator) validateVersionDir(ctx context.Context, version string) {
	invPath := filepath.Join(v.root, version, inventoryFileName)
	inv, err := ReadValidateInventory(invPath)
	if os.IsNotExist(err) {
		log.Printf(`WARNING: Version %s has not inventory`, version)
	} else if err != nil {
		v.handleErr(ctx, err)
	} else {
		v.validateInventory(ctx, &inv)
	}
	// Check version content present in manifest
	contPath := filepath.Join(v.root, version, `content`)
	walk := func(path string, info os.FileInfo, err error) error {
		if err != nil {

			return err
		}
		if done(ctx) {
			return errors.New(`stop walk`)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		ePath, pathErr := filepath.Rel(v.root, path)
		if pathErr != nil {
			return pathErr
		}
		if v.inventory.Manifest.GetDigest(ePath) == `` {
			v.handleErr(ctx, NewErrf(ManPathErr, `not in manifest: %s`, ePath))
		}
		return nil
	}
	filepath.Walk(contPath, walk)
}

// validateContentMap checks
func (v *Validator) validateContentMap(ctx context.Context, cm ContentMap, alg string) int {
	var checked int
	in := make(chan checksumJob)
	go func() {
		for dp := range cm.Iterate() {
			select {
			case <-ctx.Done():
				// drain cm Iterate
			default:
				in <- checksumJob{
					path:     filepath.Join(v.root, dp.Path),
					alg:      alg,
					expected: string(dp.Digest),
				}
			}

		}
		close(in)
	}()
	for result := range digester(ctx, in) {
		if result.err != nil {
			v.handleErr(ctx, result.err)
			continue
		}
		if result.sum != result.expected {
			v.handleErr(ctx, NewErr(ContentChecksumErr, nil))
		} else {
			checked++
		}
	}
	return checked
}

// validateInventory really just checks consistency of the inventory
func (v *Validator) validateInventory(ctx context.Context, inv *Inventory) bool {

	// Validate Inventory Structure:
	if inv.ID == `` {
		if v.handleErr(ctx, NewErr(InvIDErr, nil)) != nil {
			return false
		}
	}
	if inv.Type != inventoryType {
		if v.handleErr(ctx, NewErr(InvTypeErr, nil)) != nil {
			return false
		}
	}
	if inv.DigestAlgorithm == `` {
		if v.handleErr(ctx, NewErr(InvDigestErr, nil)) != nil {
			return false
		}
	} else if !stringIn(inv.DigestAlgorithm, digestAlgorithms[:]) {
		if v.handleErr(ctx, NewErr(InvDigestErr, nil)) != nil {
			return false
		}
	}
	if inv.Manifest == nil {
		if v.handleErr(ctx, NewErr(InvNoManErr, nil)) != nil {
			return false
		}
	}
	if inv.Versions == nil {
		if v.handleErr(ctx, NewErr(InvNoVerErr, nil)) != nil {
			return false
		}
	}
	// Validate Version Names in Inventory
	var versions = inv.versionNames()
	var padding int
	if len(inv.Versions) > 0 {
		padding = versionPadding(versions[0])
		for i := range versions {
			n, _ := versionGen(i+1, padding)
			if _, ok := inv.Versions[n]; !ok {
				if v.handleErr(ctx, NewErr(VerFormatErr, nil)) != nil {
					return false
				}
			}
		}
	}
	// make sure every digest in version state is present in the manifest
	for vname := range inv.Versions {
		for digest := range inv.Versions[vname].State {
			if inv.Manifest.LenDigest(digest) == 0 {
				if v.handleErr(ctx, NewErr(ManDigestErr, nil)) != nil {
					return false
				}
			}
		}
	}
	return true
}
