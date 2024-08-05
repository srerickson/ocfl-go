package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/extension"
	"github.com/srerickson/ocfl-go/ocflv1/codes"
)

func ValidateObject(ctx context.Context, fsys ocfl.FS, root string, vops ...ocfl.ValidationOption) (*ReadObject, *ocfl.Validation) {
	v := ocfl.NewValidation(vops...)
	// read root contents
	rootState, err := ocfl.GetObjectRoot(ctx, fsys, root)
	if err != nil {
		v.AddFatal(err)
	}
	validateRootState(ctx, rootState, v)
	validateExtensionsDir(ctx, rootState, v)
	var obj *ReadObject
	if v.Err() != nil {
		obj = &ReadObject{fs: fsys, path: root}
	}
	return obj, v
}

// validateRootState fully validates the object root contents
func validateRootState(ctx context.Context, root *ocfl.ObjectRoot, vldr *ocfl.Validation) {
	ocflV := root.State.Spec
	if err := root.ValidateNamaste(ctx, ocflV); err != nil {
		err = ec(err, codes.E007(ocflV))
		vldr.AddFatal(err)
	}
	for _, name := range root.State.Invalid {
		err := fmt.Errorf(`%w: %s`, ErrObjRootStructure, name)
		vldr.AddFatal(ec(err, codes.E001(ocflV)))
	}
	if !root.State.HasInventory() {
		err := fmt.Errorf(`%w: not found`, ErrInventoryNotExist)
		vldr.AddFatal(ec(err, codes.E063(ocflV)))
	}
	if !root.State.HasSidecar() {
		err := fmt.Errorf(`inventory sidecar: %w`, fs.ErrNotExist)
		vldr.AddFatal(ec(err, codes.E058(ocflV)))
	}
	err := root.State.VersionDirs.Valid()
	if err != nil {
		if errors.Is(err, ocfl.ErrVerEmpty) {
			err = ec(err, codes.E008(ocflV))
		} else if errors.Is(err, ocfl.ErrVNumPadding) {
			err = ec(err, codes.E011(ocflV))
		} else if errors.Is(err, ocfl.ErrVNumMissing) {
			err = ec(err, codes.E010(ocflV))
		}
		vldr.AddFatal(err)
	}
	if err == nil && root.State.VersionDirs.Padding() > 0 {
		err := errors.New("version directory names are zero-padded")
		vldr.AddWarn(ec(err, codes.W001(ocflV)))
	}
	return
}

// func (vldr *objectValidator) validateNamaste(ctx context.Context) error {
// 	ocflV := vldr.root.State.Spec
// 	lgr := vldr.opts.Logger
// 	if vldr.rootInfo.Declaration.Type != ocfl.DeclObject {
// 		err := fmt.Errorf("%w: %s", ErrOCFLVersion, ocflV)
// 		vldr.LogFatal(lgr, ec(err, codes.E004(ocflV)))
// 	}
// 	err := ocfl.ValidateDeclaration(ctx, vldr.FS, path.Join(vldr.Root, vldr.root.Declaration.Name()))
// 	if err != nil {
// 		err = ec(err, codes.E007(ocflV))
// 		vldr.LogFatal(lgr, err)
// 	}
// 	return vldr.Err()
// }

func validateRootInventory(ctx context.Context) error {
	ocflV := vldr.root.State.Spec
	lgr := vldr.opts.Logger
	name := path.Join(vldr.Root, inventoryFile)
	opts := []ValidationOption{
		copyValidationOptions(vldr.opts),
		appendResult(vldr.Result),
		FallbackOCFL(vldr.root.State.Spec),
	}
	if lgr != nil {
		opts = append(opts, ValidationLogger(lgr.With("inventory_file", name)))
	}
	inv, r := ValidateInventory(ctx, vldr.FS, name, ocflV)
	vldr.AddFatal(r.Err())
	vldr.AddWarn(r.WarnErr())
	if err := vldr.Err(); err != nil {
		return err
	}
	// Inventory head/versions are consitent with Object Root
	if expHead := vldr.root.State.VersionDirs.Head(); expHead != inv.Head {
		vldr.LogFatal(lgr, ec(fmt.Errorf("inventory head is not %s", expHead), codes.E040(inv.Type.Spec)))
		vldr.LogFatal(lgr, ec(fmt.Errorf("inventory versions don't include %s", expHead), codes.E046(inv.Type.Spec)))
	}
	// inventory has same OCFL version as declaration
	if inv.Type.Spec != ocflV {
		err := fmt.Errorf("inventory declares OCFL version %s, NAMASTE declares %s", inv.Type.Spec, ocflV)
		vldr.LogFatal(lgr, ec(err, codes.E038(ocflV)))
	}
	// add root inventory manifest to digest ledger
	if err := vldr.ledger.addInventory(inv, true); err != nil {
		// err indicates inventory includes different digest for a previously added file
		vldr.LogFatal(lgr, ec(err, codes.E066(ocflV)))
	}
	if err := vldr.Result.Err(); err != nil {
		return err
	}
	vldr.rootInv = inv
	return nil
}

func validateExtensionsDir(ctx context.Context, root *ocfl.ObjectRoot, vldr *ocfl.Validation) {
	ocflV := root.State.Spec
	extDir := path.Join(root.Path, extensionsDir)
	items, err := root.FS.ReadDir(ctx, extDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		vldr.AddFatal(err)
	}
	for _, i := range items {
		if !i.IsDir() {
			err := fmt.Errorf(`unexpected file: %s`, i.Name())
			vldr.AddFatal(ec(err, codes.E067(ocflV)))
			continue
		}
		_, err := extension.Get(i.Name())
		if err != nil {
			// unknow extension
			vldr.AddWarn(ec(fmt.Errorf("%w: %s", err, i.Name()), codes.W013(ocflV)))
		}
	}
}

// validatePathLedger validates the pathLedger. Before running
// validatePathLedger, all inventories in the object should have been added to
// the ledger (with addInventory) and all version content should have been
// indexed (with indexVersionContent). The ledger is valid if (1) every path
// exists, (2) every path exists in the root inventory manifest, (3) every path
// exists in version inventory manifests equal of greater than the version in
// which the path is stored, (4) all digests for all paths are confirmed.
func (vldr *objectValidator) validatePathLedger(ctx context.Context) error {
	ocflV := vldr.root.State.Spec
	lgr := vldr.opts.Logger
	// check paths exist are in included in manifsts as necessary
	for p, pInfo := range vldr.ledger.paths {
		pVer := pInfo.existsIn // version wheren content file is stored (or empty ocfl.Num)
		if pVer.IsZero() {
			for v, f := range pInfo.locations() {
				locStr := "root"
				if !f.InRoot() {
					locStr = v.String()
				}
				if f.InManifest() {
					err := fmt.Errorf("path referenced in %s inventory manifest does not exist: %s", locStr, p)
					vldr.LogFatal(lgr, ec(err, codes.E092(ocflV)))
				}
				if f.InFixity() {
					err := fmt.Errorf("path referenced in %s inventory fixity does not exist: %s", locStr, p)
					vldr.LogFatal(lgr, ec(err, codes.E093(ocflV)))
				}
			}
		}
		for v := range vldr.ledger.inventories {
			if v.Num() >= pVer.Num() {
				if !pInfo.referencedIn(v, inManifest) {
					err := fmt.Errorf("path not referenecd in %s manifest as expected: %s", v, p)
					vldr.LogFatal(lgr, ec(err, codes.E023(ocflV)))
				}
			}
		}
	}
	// don't continue if there are previous errors
	if err := vldr.Err(); err != nil {
		return err
	}
	// don't continue if NoDigest is set
	if vldr.opts.SkipDigests {
		return nil
	}
	// digests
	var setupErr error
	digestSetup := func(digestFile func(name string, algs []string) bool) {
		for name, pInfo := range vldr.ledger.paths {
			algs := make([]string, 0, len(pInfo.digests))
			for alg := range pInfo.digests {
				algs = append(algs, alg)
			}
			if len(algs) == 0 {
				// no digests associate with the path
				err := fmt.Errorf("path not referenecd in manifest as expected: %s", name)
				setupErr = ec(err, codes.E023(ocflV))
				return
			}
			if !digestFile(path.Join(vldr.Root, name), algs) {
				setupErr = errors.New("digest validation interupted")
			}
		}
	}
	var digestErr error
	ocfl.Digest(ctx, vldr.FS, digestSetup)(func(r ocfl.DigestResult, err error) bool {
		if err != nil {
			digestErr = err
			if errors.Is(digestErr, fs.ErrNotExist) {
				digestErr = ec(digestErr, codes.E092(ocflV))
			}
			vldr.LogFatal(lgr, digestErr)
			return false
		}
		name := r.Path
		for alg, sum := range r.Digests {
			// convert path back from FS-relative to object-relative path
			objPath := strings.TrimPrefix(name, vldr.Root+"/")
			entry, exists := vldr.ledger.getDigest(objPath, alg)
			if !exists {
				panic(`BUG: path/algorithm not a valid key as expected`)
			}
			if !strings.EqualFold(sum, entry.digest) {
				digestErr = &ContentDigestErr{
					Path:   name,
					Alg:    alg,
					Entry:  *entry,
					Digest: sum,
				}
				for _, l := range entry.locs {
					if l.InManifest() {
						vldr.LogFatal(lgr, ec(digestErr, codes.E092(ocflV)))
					} else {
						vldr.LogFatal(lgr, ec(digestErr, codes.E093(ocflV)))
					}
				}
				return false
			}
		}
		return true
	})
	if err := errors.Join(setupErr, digestErr); err != nil {
		vldr.LogFatal(lgr, err)
		return err
	}
	return nil
}
