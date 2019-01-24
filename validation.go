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
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// Validator handles state for OCFL Object validation
type Validator struct {
	root      string
	inventory *Inventory
	// ctx       context.Context
	Cancel  context.CancelFunc
	errChan chan Error
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
		oErr, ok := err.(ObjectError)
		if !ok {
			panic(`tried to send non-ObjectError to Validator`)
		}
		v.errChan <- oErr
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
func ValidateObject(path string) Error {
	var v = Validator{}
	var ctx context.Context
	ctx, v.Cancel = context.WithCancel(context.Background())
	for vErr := range v.ValidateObject(ctx, path) {
		// cancel remaining validation on first error
		v.Cancel()
		return vErr
	}
	return nil
}

// ValidateObject validates OCFL object located at path
func (v *Validator) ValidateObject(ctx context.Context, path string) chan Error {
	v.errChan = make(chan Error)
	go func() {
		defer close(v.errChan)

		// Load Object
		obj, err := GetObject(path)
		if err != nil {
			v.handleErr(ctx, err)
			return
		}
		v.validateInventory(ctx, &(obj.inventory))

		// v.root = obj.Path
		// v.inventory = &obj.inventory
		// alg := v.inventory.DigestAlgorithm

		// // Validate Each Version Directory
		// files, ioErr := ioutil.ReadDir(path)
		// if ioErr != nil && v.handleErr(ctx, NewErr(ReadErr, ioErr)) != nil {
		// 	return
		// }
		// for _, f := range files {
		// 	// check if context is canceled
		// 	if done(ctx) {
		// 		break
		// 	}
		// 	if !f.IsDir() {
		// 		continue
		// 	}
		// 	if style := versionFormat(f.Name()); style != `` {
		// 		v.validateVersionDir(ctx, f.Name())
		// 	}
		// }
		// // Manifest Checksum
		// if v.inventory.Manifest.Validate(v.root, alg); err != nil {
		// 	retErr = err
		// }
		// // Fixity Checksum
		// for alg, manifest := range v.inventory.Fixity {
		// 	if err := manifest.Validate(v.root, alg); err != nil {
		// 		retErr = err
		// 	}
		// }

	}()
	return v.errChan
}

func (v *Validator) validateVersionDir(ctx context.Context, version string) error {
	invPath := filepath.Join(v.root, version, inventoryFileName)
	_, retErr := ReadValidateInventory(invPath)
	if os.IsNotExist(retErr) {
		log.Printf(`WARNING: Version %s has not inventory`, version)
	} else if retErr != nil {
		return retErr
	}
	// Check version content present in manifest
	contPath := filepath.Join(v.root, version, `content`)
	walk := func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		ePath, pathErr := filepath.Rel(v.root, path)
		if pathErr != nil {
			return pathErr
		}
		if v.inventory.Manifest.GetDigest(ePath) == `` {
			retErr = fmt.Errorf(`not in manifest: %s`, ePath)
			log.Print(retErr)
		}
		return nil
	}
	if err := filepath.Walk(contPath, walk); err != nil && !os.IsNotExist(err) {
		return err
	}
	return retErr
}

func (v *Validator) validateContentMap(ctx context.Context, cm ContentMap) {

}

func (v *Validator) validateInventory(ctx context.Context, inv *Inventory) {

	// Validate Inventory Structure:
	if inv.ID == `` {
		if v.handleErr(ctx, NewErr(InvIDErr, nil)) != nil {
			return
		}
	}
	if inv.Type != inventoryType {
		if v.handleErr(ctx, NewErr(InvTypeErr, nil)) != nil {
			return
		}
	}
	if inv.DigestAlgorithm == `` {
		if v.handleErr(ctx, NewErr(InvDigestErr, nil)) != nil {
			return
		}
	} else if !stringIn(inv.DigestAlgorithm, digestAlgorithms[:]) {
		if v.handleErr(ctx, NewErr(InvDigestErr, nil)) != nil {
			return
		}
	}
	if inv.Manifest == nil {
		if v.handleErr(ctx, NewErr(InvNoManErr, nil)) != nil {
			return
		}
	}
	if inv.Versions == nil {
		if v.handleErr(ctx, NewErr(InvNoVerErr, nil)) != nil {
			return
		}
	}
	// Validate Version Names in Inventory
	var versions = inv.versionNames()
	var padding int
	if len(inv.Versions) > 0 {
		padding = versionPadding(versions[0])
		for i := range versions {
			// if done(ctx){
			// 	return
			// }
			n, _ := versionGen(i+1, padding)
			if _, ok := inv.Versions[n]; !ok {
				if v.handleErr(ctx, NewErr(VerFormatErr, nil)) != nil {
					return
				}
			}
		}
	}
	// make sure every digest in version state is present in the manifest
	for vname := range inv.Versions {
		for digest := range inv.Versions[vname].State {
			if inv.Manifest.LenDigest(digest) == 0 {
				if v.handleErr(ctx, NewErr(ManDigestErr, nil)) != nil {
					return
				}
			}
		}
	}
}
