package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"reflect"
	"strings"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/extension"
	"github.com/srerickson/ocfl-go/ocflv1/codes"
	"github.com/srerickson/ocfl-go/validation"
)

func ValidateObject(ctx context.Context, fsys ocfl.FS, root string, vops ...ocfl.ValidationOption) (*ReadObject, *ocfl.Validation) {
	v := ocfl.NewValidation(vops...)
	vldr := objectValidator{
		Result: validation.NewResult(-1),
		opts: &validationOptions{
			Logger: v.Logger(),
			// SkipDigests:  v.SkipDigests(),
			FallbackOCFL: ocfl.Spec1_1,
		},
		FS:       fsys,
		Root:     root,
		ledger:   &pathLedger{},
		verSpecs: map[ocfl.VNum]ocfl.Spec{},
	}
	vldr.validate(ctx)
	result := &ocfl.Validation{}
	result.AddFatal(vldr.Result.Fatal()...)
	result.AddWarn(vldr.Result.Warn()...)
	if err := result.Err(); err != nil {
		return nil, result
	}
	obj := &ReadObject{fs: fsys, path: root, inv: vldr.rootInv}
	return obj, result
}

// objectValidator represents state of an object validation process
type objectValidator struct {
	*validation.Result
	opts *validationOptions
	FS   ocfl.FS // required
	Root string  // required

	// entries belows are state set during validation
	rootInv  *RawInventory // TODO: remove, state instead
	root     *ocfl.ObjectRoot
	ledger   *pathLedger
	verSpecs map[ocfl.VNum]ocfl.Spec
}

// ValidateObject performs complete validation of the object at path p in fsys.
func (vldr *objectValidator) validate(ctx context.Context) *validation.Result {
	lgr := vldr.opts.Logger
	obj, err := ocfl.GetObjectRoot(ctx, vldr.FS, vldr.Root)
	if err != nil {
		return vldr.LogFatal(lgr, err)
	}
	vldr.root = obj
	ocflV := vldr.root.State.Spec
	switch ocflV {
	case ocfl.Spec1_0:
		fallthrough
	case ocfl.Spec1_1:
		if err := vldr.validateRoot(ctx); err != nil {
			return vldr.Result
		}
		for _, vnum := range vldr.root.State.VersionDirs {
			if err := vldr.validateVersion(ctx, vnum); err != nil {
				return vldr.Result
			}
			// check version inventory uses OCFL spec that is >= spec used in previous inventory
			if vnumSpec, exists := vldr.verSpecs[vnum]; exists {
				var prevVer = vnum
				for {
					var err error
					prevVer, err = prevVer.Prev()
					if err != nil {
						break
					}
					if prevSpec, exists := vldr.verSpecs[prevVer]; exists {
						if prevSpec.Cmp(vnumSpec) > 0 {
							err := fmt.Errorf("%s uses a lower version of the OCFL spec than %s (%s < %s)", vnum, prevVer, vnumSpec, prevSpec)
							vldr.LogFatal(lgr, ec(err, codes.E103(ocflV)))
						}
						break
					}
				}
			}
		}
		if err := vldr.validateExtensionsDir(ctx); err != nil {
			return vldr.Result
		}
		if err := vldr.validatePathLedger(ctx); err != nil {
			return vldr.Result
		}
	default:
		err := fmt.Errorf("%w: %s", ErrOCFLVersion, ocflV)
		return vldr.LogFatal(lgr, err)
	}
	return vldr.Result
}

// validateRoot fully validates the object root contents
func (vldr *objectValidator) validateRoot(ctx context.Context) error {
	ocflV := vldr.root.State.Spec
	lgr := vldr.opts.Logger
	if err := vldr.root.ValidateNamaste(ctx, ocflV); err != nil {
		err = ec(err, codes.E007(ocflV))
		vldr.LogFatal(lgr, err)
	}
	for _, name := range vldr.root.State.Invalid {
		err := fmt.Errorf(`%w: %s`, ErrObjRootStructure, name)
		vldr.LogFatal(lgr, ec(err, codes.E001(ocflV)))
	}
	if !vldr.root.State.HasInventory() {
		err := fmt.Errorf(`%w: not found`, ErrInventoryNotExist)
		vldr.LogFatal(lgr, ec(err, codes.E063(ocflV)))
	}
	if !vldr.root.State.HasSidecar() {
		err := fmt.Errorf(`inventory sidecar: %w`, fs.ErrNotExist)
		vldr.LogFatal(lgr, ec(err, codes.E058(ocflV)))
	}
	err := vldr.root.State.VersionDirs.Valid()
	if err != nil {
		if errors.Is(err, ocfl.ErrVerEmpty) {
			err = ec(err, codes.E008(ocflV))
		} else if errors.Is(err, ocfl.ErrVNumPadding) {
			err = ec(err, codes.E011(ocflV))
		} else if errors.Is(err, ocfl.ErrVNumMissing) {
			err = ec(err, codes.E010(ocflV))
		}
		vldr.LogFatal(lgr, err)
	}
	if err == nil && vldr.root.State.VersionDirs.Padding() > 0 {
		err := errors.New("version directory names are zero-padded")
		vldr.LogWarn(lgr, ec(err, codes.W001(ocflV)))
	}
	return vldr.validateRootInventory(ctx)
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

func (vldr *objectValidator) validateRootInventory(ctx context.Context) error {
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

func (vldr *objectValidator) validateVersion(ctx context.Context, ver ocfl.VNum) error {
	ocflV := vldr.root.State.Spec // assumed ocfl version (until inventory is decoded)
	lgr := vldr.opts.Logger
	if lgr != nil {
		lgr = lgr.With("version", ver.String())
	}
	vDir := path.Join(vldr.Root, ver.String())
	entries, err := vldr.FS.ReadDir(ctx, vDir)
	if err != nil {
		vldr.LogFatal(lgr, err)
		return err
	}
	info := parseVersionDirState(entries)
	for _, f := range info.extraFiles {
		err := fmt.Errorf(`unexpected file in %s: %s`, ver, f)
		vldr.LogFatal(lgr, ec(err, codes.E015(ocflV)))
	}
	for _, d := range info.dirs {
		// directory must be content directory
		if contDir := vldr.rootInv.ContentDirectory; d == contDir {
			// add version content directory to validation state
			added, err := vldr.walkVersionContent(ctx, ver)
			if err != nil {
				vldr.LogFatal(lgr, err)
				return err
			}
			if added == 0 {
				// content directory exists but it's empty
				err := fmt.Errorf("content directory (%s) contains no files", contDir)
				vldr.LogFatal(lgr, ec(err, codes.E016(ocflV)))
			}
			continue
		}
		err := fmt.Errorf(`extra directory in %s: %s`, ver, d)
		vldr.LogWarn(lgr, ec(err, codes.W002(ocflV)))
	}
	if info.hasInventory {
		// if !algorithms[info.] {
		// 	err := fmt.Errorf("%w: %s", ErrDigestAlg, info.asidecarAlg
		// 	vldr.LogFatal(lgr, ec(err, codes.E025(ocflV)))
		// }
		if err := vldr.validateVersionInventory(ctx, ver); err != nil {
			return err
		}
	} else {
		vldr.LogWarn(lgr, ec(errors.New("missing version inventory"), codes.W010(ocflV)))
	}
	return vldr.Err()
}

func validateVersionInventory(ctx context.Context, vldr *ocfl.Validation, dirInv *RawInventory, prev ocfl.Inventory, rootInv ocfl.Inventory) error {

	// Is this the HEAD version directory?
	if dirInv.Head == rootInv.Head() {
		if dirInv.jsonDigest == rootInv.jsonDigest {
			return nil // don't need to validate any further
		}
		err := fmt.Errorf("inventory in last version (%s) is not same as root inventory", dirInv.Head.String())
		vldr.AddFatal(ec(err, codes.E064(dirInv.Type.Spec)))
	}
	//
	// remaining validations should check consistency between version inventory
	// and root inventory
	//
	// check expected values specified in conf
	if rootInv.ID() != dirInv.ID {
		err := fmt.Errorf("unexpected id: %s", dirInv.ID)
		vldr.AddFatal(ec(err, codes.E037(dirInv.Type.Spec)))
	}
	if rootInv.ContentDirectory() != dirInv.ContentDirectory {
		err := fmt.Errorf("contentDirectory is '%s', but expected '%s'", dirInv.ContentDirectory, rootInv.ContentDirectory())
		vldr.AddFatal(ec(err, codes.E019(dirInv.Type.Spec)))
	}
	if vn != dirInv.Head {
		err := fmt.Errorf("inventory head is %s, expected %s", dirInv.Head, vn)
		vldr.LogFatal(lgr, ec(err, codes.E040(dirInv.Type.Spec)))
	}
	// confirm that each version's logical state in the version directory
	// inventory matches the corresponding logical state in the root inventory.
	// Because the digest algorithm can change between versions, we're comparing
	// the set of logical paths, and their correspondance to
	for v, ver := range dirInv.Versions {
		rootVer := rootInv.Version(v.Num())
		rootLogicalState := rootInv.logicalState(v.Num())
		dirLogicalState := dirInv.logicalState(v.Num())
		if !dirLogicalState.Eq(rootLogicalState) {
			err := fmt.Errorf("logical state for version %d in root inventory doesn't match that in %s/%s", v.Num(), vn, inventoryFile)
			vldr.LogFatal(lgr, ec(err, codes.E066(dirInv.Type.Spec)))
		}
		if ver.Message != rootVer.Message {
			err := fmt.Errorf(`message for version %s differs from root inventory`, v)
			vldr.LogWarn(lgr, ec(err, codes.W011(dirInv.Type.Spec)))
		}
		if !reflect.DeepEqual(ver.User, rootVer.User) {
			err := fmt.Errorf(`user information for version %s differs from root inventory`, v)
			vldr.LogWarn(lgr, ec(err, codes.W011(dirInv.Type.Spec)))
		}
		if ver.Created != rootVer.Created {
			err := fmt.Errorf(`timestamp for version %s differs from root inventory`, v)
			vldr.LogWarn(lgr, ec(err, codes.W011(dirInv.Type.Spec)))
		}
	}
	return vldr.Err()
}

func (vldr *objectValidator) validateExtensionsDir(ctx context.Context) error {
	lgr := vldr.opts.Logger
	extDir := path.Join(vldr.Root, extensionsDir)
	items, err := vldr.FS.ReadDir(ctx, extDir)
	if lgr != nil {
		lgr = lgr.With("dir", extensionsDir)
	}
	ocflV := vldr.root.State.Spec
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		vldr.LogFatal(lgr, err)
		return err
	}
	for _, i := range items {
		if !i.IsDir() {
			err := fmt.Errorf(`unexpected file: %s`, i.Name())
			vldr.LogFatal(lgr, ec(err, codes.E067(ocflV)))
			continue
		}
		_, err := extension.Get(i.Name())
		if err != nil {
			// unknow extension
			vldr.LogWarn(lgr, ec(fmt.Errorf("%w: %s", err, i.Name()), codes.W013(ocflV)))
		}
	}
	return nil
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

func (vldr *objectValidator) walkVersionContent(ctx context.Context, ver ocfl.VNum) (int, error) {
	contDir := path.Join(vldr.Root, ver.String(), vldr.rootInv.ContentDirectory)
	var added int
	var iterErr error
	ocfl.Files(ctx, vldr.FS, contDir)(func(info ocfl.FileInfo, err error) bool {
		if err != nil {
			iterErr = err
			return false
		}
		// convert fs-relative path to object-relative path
		name := info.Path
		objPath := strings.TrimPrefix(name, vldr.Root+"/")
		vldr.ledger.addPathExists(objPath, ver)
		added++
		return true
	})
	return added, iterErr
}
