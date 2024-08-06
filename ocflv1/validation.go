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
)

func ValidateObject(ctx context.Context, fsys ocfl.FS, dir string, vops ...ocfl.ValidationOption) (*ReadObject, *ocfl.Validation) {
	v := ocfl.NewValidation(vops...)
	// read root contents
	root, err := ocfl.GetObjectRoot(ctx, fsys, dir)
	if err != nil {
		v.AddFatal(err)
		return nil, v
	}
	ocflV := root.State.Spec
	if err := root.ValidateNamaste(ctx, ocflV); err != nil {
		err = ec(err, codes.E007(ocflV))
		v.AddFatal(err)
	}
	if err := validateRootState(root.State, v); err != nil {
		return nil, v
	}
	inv, err := ValidateInventory(ctx, fsys, path.Join(dir, inventoryFile), v)
	if err != nil {
		return nil, v
	}
	// inventory has same OCFL version as declaration
	if inv.Type.Spec != ocflV {
		err := fmt.Errorf("inventory declares OCFL version %s, NAMASTE declares %s", inv.Type.Spec, ocflV)
		v.AddFatal(ec(err, codes.E038(ocflV)))
	}
	// Inventory head/versions are consitent with Object Root
	if expHead := root.State.VersionDirs.Head(); expHead != inv.Head {
		v.AddFatal(ec(fmt.Errorf("root inventory 'head' is not %s", expHead), codes.E040(ocflV)))
		// v.AddFatal(ec(fmt.Errorf("inventory versions don't include %s", expHead), codes.E046(ocflV)))
	}
	validateExtensionsDir(ctx, root, v)
	if v.Err() != nil {
		return nil, v
	}
	obj := &ReadObject{fs: fsys, path: dir, inv: inv}
	var prev ocfl.ReadInventory
	for _, vnum := range inv.Head.AsHead() {
		prev = validateVersion(ctx, obj, vnum, prev, v)
	}
	return obj, v
}

// validateRootState fully validates the object root contents
func validateRootState(state *ocfl.ObjectRootState, vldr *ocfl.Validation) error {
	ocflV := state.Spec
	for _, name := range state.Invalid {
		err := fmt.Errorf(`%w: %s`, ErrObjRootStructure, name)
		vldr.AddFatal(ec(err, codes.E001(ocflV)))
	}
	if !state.HasInventory() {
		err := fmt.Errorf(`%w: not found`, ErrInventoryNotExist)
		vldr.AddFatal(ec(err, codes.E063(ocflV)))
	}
	if !state.HasSidecar() {
		err := fmt.Errorf(`inventory sidecar: %w`, fs.ErrNotExist)
		vldr.AddFatal(ec(err, codes.E058(ocflV)))
	}
	err := state.VersionDirs.Valid()
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
	if err == nil && state.VersionDirs.Padding() > 0 {
		err := errors.New("version directory names are zero-padded")
		vldr.AddWarn(ec(err, codes.W001(ocflV)))
	}
	return vldr.Err()
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
// func (vldr *objectValidator) validatePathLedger(ctx context.Context) error {
// 	ocflV := vldr.root.State.Spec
// 	lgr := vldr.opts.Logger
// 	// check paths exist are in included in manifsts as necessary
// 	for p, pInfo := range vldr.ledger.paths {
// 		pVer := pInfo.existsIn // version wheren content file is stored (or empty ocfl.Num)
// 		if pVer.IsZero() {
// 			for v, f := range pInfo.locations() {
// 				locStr := "root"
// 				if !f.InRoot() {
// 					locStr = v.String()
// 				}
// 				if f.InManifest() {
// 					err := fmt.Errorf("path referenced in %s inventory manifest does not exist: %s", locStr, p)
// 					vldr.LogFatal(lgr, ec(err, codes.E092(ocflV)))
// 				}
// 				if f.InFixity() {
// 					err := fmt.Errorf("path referenced in %s inventory fixity does not exist: %s", locStr, p)
// 					vldr.LogFatal(lgr, ec(err, codes.E093(ocflV)))
// 				}
// 			}
// 		}
// 		for v := range vldr.ledger.inventories {
// 			if v.Num() >= pVer.Num() {
// 				if !pInfo.referencedIn(v, inManifest) {
// 					err := fmt.Errorf("path not referenecd in %s manifest as expected: %s", v, p)
// 					vldr.LogFatal(lgr, ec(err, codes.E023(ocflV)))
// 				}
// 			}
// 		}
// 	}
// 	// don't continue if there are previous errors
// 	if err := vldr.Err(); err != nil {
// 		return err
// 	}
// 	// don't continue if NoDigest is set
// 	if vldr.opts.SkipDigests {
// 		return nil
// 	}
// 	// digests
// 	var setupErr error
// 	digestSetup := func(digestFile func(name string, algs []string) bool) {
// 		for name, pInfo := range vldr.ledger.paths {
// 			algs := make([]string, 0, len(pInfo.digests))
// 			for alg := range pInfo.digests {
// 				algs = append(algs, alg)
// 			}
// 			if len(algs) == 0 {
// 				// no digests associate with the path
// 				err := fmt.Errorf("path not referenecd in manifest as expected: %s", name)
// 				setupErr = ec(err, codes.E023(ocflV))
// 				return
// 			}
// 			if !digestFile(path.Join(vldr.Root, name), algs) {
// 				setupErr = errors.New("digest validation interupted")
// 			}
// 		}
// 	}
// 	var digestErr error
// 	ocfl.Digest(ctx, vldr.FS, digestSetup)(func(r ocfl.DigestResult, err error) bool {
// 		if err != nil {
// 			digestErr = err
// 			if errors.Is(digestErr, fs.ErrNotExist) {
// 				digestErr = ec(digestErr, codes.E092(ocflV))
// 			}
// 			vldr.LogFatal(lgr, digestErr)
// 			return false
// 		}
// 		name := r.Path
// 		for alg, sum := range r.Digests {
// 			// convert path back from FS-relative to object-relative path
// 			objPath := strings.TrimPrefix(name, vldr.Root+"/")
// 			entry, exists := vldr.ledger.getDigest(objPath, alg)
// 			if !exists {
// 				panic(`BUG: path/algorithm not a valid key as expected`)
// 			}
// 			if !strings.EqualFold(sum, entry.digest) {
// 				digestErr = &ContentDigestErr{
// 					Path:   name,
// 					Alg:    alg,
// 					Entry:  *entry,
// 					Digest: sum,
// 				}
// 				for _, l := range entry.locs {
// 					if l.InManifest() {
// 						vldr.LogFatal(lgr, ec(digestErr, codes.E092(ocflV)))
// 					} else {
// 						vldr.LogFatal(lgr, ec(digestErr, codes.E093(ocflV)))
// 					}
// 				}
// 				return false
// 			}
// 		}
// 		return true
// 	})
// 	if err := errors.Join(setupErr, digestErr); err != nil {
// 		vldr.LogFatal(lgr, err)
// 		return err
// 	}
// 	return nil
// }

// validateVersion fully validates an ocfl v1.x version directory for ver in the object.
// the previous inventory may be nil.
func validateVersion(ctx context.Context, obj ocfl.ReadObject, dirNum ocfl.VNum, prev ocfl.ReadInventory, vldr *ocfl.Validation) ocfl.ReadInventory {
	fsys := obj.FS()
	vDir := path.Join(obj.Path(), dirNum.String())
	// isHead := obj.Inventory().Head() == dirNum
	verContentDir := contentDir
	// what spec does the version use? Priority:
	//  1. spec in inventory (if present)
	//  2. prev spec (if present)
	//  3. 1.0
	verSpec := ocfl.Spec1_0
	if prev != nil {
		verSpec = prev.Spec()
	}
	// read the contents of the version directory
	entries, err := fsys.ReadDir(ctx, vDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		vldr.AddFatal(err)
		return nil
	}
	if len(entries) < 1 {
		// the version directory doesn't exist or it's empty
		err := fmt.Errorf("missing %s/inventory.json", dirNum.String())
		vldr.AddWarn(ec(err, codes.W010(verSpec)))
		return nil
	}
	info := parseVersionDirState(entries)
	for _, f := range info.extraFiles {
		err := fmt.Errorf(`unexpected file in %s: %s`, dirNum, f)
		vldr.AddFatal(ec(err, codes.E015(verSpec)))
	}
	var versionInv ocfl.ReadInventory
	if info.hasInventory {
		invName := path.Join(vDir, inventoryFile)
		inv, err := ValidateInventory(ctx, fsys, invName, vldr)
		if err == nil {
			verSpec = versionInv.Spec()
			verContentDir = versionInv.ContentDirectory()
		}
		// would be nice to add prefix to these errors
		if inv != nil && prev != nil {
			// err := fmt.Errorf("%s uses a lower version of the OCFL spec than %s (%s < %s)", vnum, prevVer, vnumSpec, prevSpec)
			// vldr.LogFatal(lgr, ec(err, codes.E103(ocflV)))

			// the version content directory must be the same
			// the ocfl spec must >=
			// check that all version states in prev match the corresponding
			// version state in this inventory
			for _, v := range prev.Head().AsHead() {
				versionThis := versionInv.Version(v.Num())
				versionPrev := prev.Version(v.Num())
				vLogicalStateThis := logicalState{
					state:    versionThis.State(),
					manifest: versionInv.Manifest(),
				}
				vLogicalStatePrev := logicalState{
					state:    versionPrev.State(),
					manifest: prev.Manifest(),
				}
				if !vLogicalStateThis.Eq(vLogicalStatePrev) {
					err := fmt.Errorf("%s/inventory.json has different logical state in its %s version block than the previous inventory.json", dirNum, v)
					vldr.AddFatal(ec(err, codes.E066(verSpec)))
				}
				if versionThis.Message() != versionPrev.Message() {
					err := fmt.Errorf("%s/inventory.json has different 'message' in its %s version block than the previous inventory.json", dirNum, v)
					vldr.AddWarn(ec(err, codes.W011(verSpec)))
				}
				if !reflect.DeepEqual(versionThis.User, versionPrev.User()) {
					err := fmt.Errorf("%s/inventory.json has different 'user' in its %s version block than the previous inventory.json", dirNum, v)
					vldr.AddWarn(ec(err, codes.W011(verSpec)))
				}
				if versionThis.Created() != versionPrev.Created() {
					err := fmt.Errorf("%s/inventory.json has different 'created' in its %s version block than the previous inventory.json", dirNum, v)
					vldr.AddWarn(ec(err, codes.W011(verSpec)))
				}
			}
		}
	}
	for _, d := range info.dirs {
		// directory SHOULD only be content directory
		if d != verContentDir {
			err := fmt.Errorf(`extra directory in %s: %s`, dirNum, d)
			vldr.AddWarn(ec(err, codes.W002(verSpec)))
			continue
		}
		// add version content directory to validation state
		var added int
		var iterErr error
		ocfl.Files(ctx, fsys, path.Join(vDir, verContentDir))(func(info ocfl.FileInfo, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			// convert fs-relative path to object-relative path
			vldr.AddContentExists(strings.TrimPrefix(info.Path, obj.Path()+"/"))
			added++
			return true
		})
		if iterErr != nil {
			vldr.AddFatal(err)
			return nil
		}
		if added == 0 {
			// content directory exists but it's empty
			err := fmt.Errorf("content directory (%s) contains no files", verContentDir)
			vldr.AddFatal(ec(err, codes.E016(verSpec)))
		}
	}
	if versionInv == nil {
		return nil
	}
	return versionInv
}

type versionDirState struct {
	hasInventory bool
	sidecarAlg   string
	extraFiles   []string
	dirs         []string
}

func parseVersionDirState(entries []fs.DirEntry) versionDirState {
	var info versionDirState
	for _, e := range entries {
		if e.Type().IsDir() {
			info.dirs = append(info.dirs, e.Name())
			continue
		}
		if e.Type().IsRegular() || e.Type() == fs.ModeIrregular {
			if e.Name() == inventoryFile {
				info.hasInventory = true
				continue
			}
			if strings.HasPrefix(e.Name(), inventoryFile+".") && info.sidecarAlg == "" {
				info.sidecarAlg = strings.TrimPrefix(e.Name(), inventoryFile+".")
				continue
			}
		}
		// unexpected files
		info.extraFiles = append(info.extraFiles, e.Name())
	}
	return info
}
