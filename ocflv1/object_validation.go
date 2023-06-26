package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/digest/checksum"
	"github.com/srerickson/ocfl/extensions"
	"github.com/srerickson/ocfl/ocflv1/codes"
	"github.com/srerickson/ocfl/validation"
)

func ValidateObject(ctx context.Context, fsys ocfl.FS, root string, vops ...ValidationOption) (*Object, *validation.Result) {
	opts, result := validationSetup(vops)
	vldr := objectValidator{
		Result:   result,
		opts:     opts,
		FS:       fsys,
		Root:     root,
		ledger:   &pathLedger{},
		verSpecs: map[ocfl.VNum]ocfl.Spec{},
	}
	vldr.validate(ctx)
	if err := result.Err(); err != nil {
		return nil, result
	}
	obj := &Object{ObjectRoot: *vldr.root, Inventory: *vldr.rootInv}
	return obj, result
}

// objectValidator represents state of an object validation process
type objectValidator struct {
	*validation.Result
	opts *validationOptions
	FS   ocfl.FS // required
	Root string  // required

	// entries belows are state set during validation
	rootInv  *Inventory // TODO: remove, state instead
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
	ocflV := vldr.root.Spec
	switch ocflV {
	case ocfl.Spec{1, 0}:
		fallthrough
	case ocfl.Spec{1, 1}:
		if err := vldr.validateRoot(ctx); err != nil {
			return vldr.Result
		}
		for _, vnum := range vldr.root.VersionDirs {
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
							vldr.LogFatal(lgr, ec(err, codes.E103.Ref(ocflV)))
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
		err := fmt.Errorf("%w: %s", ErrOCFLVersion, ocflV.String())
		return vldr.LogFatal(lgr, err)
	}
	return vldr.Result
}

// validateRoot fully validates the object root contents
func (vldr *objectValidator) validateRoot(ctx context.Context) error {
	ocflV := vldr.root.Spec
	lgr := vldr.opts.Logger
	if err := vldr.root.ValidateDeclaration(ctx); err != nil {
		err = ec(err, codes.E007.Ref(ocflV))
		vldr.LogFatal(lgr, err)
	}
	for _, name := range vldr.root.NonConform {
		err := fmt.Errorf(`%w: %s`, ErrObjRootStructure, name)
		vldr.LogFatal(lgr, ec(err, codes.E001.Ref(ocflV)))
	}
	if !vldr.root.HasInventory() {
		err := fmt.Errorf(`%w: not found`, ErrInventoryOpen)
		vldr.LogFatal(lgr, ec(err, codes.E063.Ref(ocflV)))
	}
	if !vldr.root.HasSidecar() {
		err := fmt.Errorf(`%w: not found`, ErrInvSidecarOpen)
		vldr.LogFatal(lgr, ec(err, codes.E058.Ref(ocflV)))
	} else if !algorithms[vldr.root.Algorithm] {
		err := fmt.Errorf(`%w: %s`, ErrDigestAlg, vldr.root.Algorithm)
		vldr.LogFatal(lgr, ec(err, codes.E025.Ref(ocflV)))
	}
	err := vldr.root.VersionDirs.Valid()
	if err != nil {
		if errors.Is(err, ocfl.ErrVerEmpty) {
			err = ec(err, codes.E008.Ref(ocflV))
		} else if errors.Is(err, ocfl.ErrVNumPadding) {
			err = ec(err, codes.E011.Ref(ocflV))
		} else if errors.Is(err, ocfl.ErrVNumMissing) {
			err = ec(err, codes.E010.Ref(ocflV))
		}
		vldr.LogFatal(lgr, err)
	}
	if err == nil && vldr.root.VersionDirs.Padding() > 0 {
		err := errors.New("version directory names are zero-padded")
		vldr.LogWarn(lgr, ec(err, codes.W001.Ref(ocflV)))
	}
	return vldr.validateRootInventory(ctx)
}

// func (vldr *objectValidator) validateNamaste(ctx context.Context) error {
// 	ocflV := vldr.root.Spec
// 	lgr := vldr.opts.Logger
// 	if vldr.rootInfo.Declaration.Type != ocfl.DeclObject {
// 		err := fmt.Errorf("%w: %s", ErrOCFLVersion, ocflV)
// 		vldr.LogFatal(lgr, ec(err, codes.E004.Ref(ocflV)))
// 	}
// 	err := ocfl.ValidateDeclaration(ctx, vldr.FS, path.Join(vldr.Root, vldr.root.Declaration.Name()))
// 	if err != nil {
// 		err = ec(err, codes.E007.Ref(ocflV))
// 		vldr.LogFatal(lgr, err)
// 	}
// 	return vldr.Err()
// }

func (vldr *objectValidator) validateRootInventory(ctx context.Context) error {
	ocflV := vldr.root.Spec
	lgr := vldr.opts.Logger
	name := path.Join(vldr.Root, inventoryFile)
	algID := vldr.root.Algorithm
	alg, err := digest.Get(algID) // use default registry b/c must be sha512/256
	if err != nil {
		vldr.LogFatal(lgr, err)
		return err
	}
	opts := []ValidationOption{
		copyValidationOptions(vldr.opts),
		appendResult(vldr.Result),
		FallbackOCFL(vldr.root.Spec),
		ValidationLogger(lgr.WithName(inventoryFile)),
	}
	inv, _ := ValidateInventory(ctx, vldr.FS, name, alg, opts...)
	if err := vldr.Err(); err != nil {
		return err
	}
	// Inventory head/versions are consitent with Object Root
	if expHead := vldr.root.VersionDirs.Head(); expHead != inv.Head {
		vldr.LogFatal(lgr, ec(fmt.Errorf("inventory head is not %s", expHead), codes.E040.Ref(inv.Type.Spec)))
		vldr.LogFatal(lgr, ec(fmt.Errorf("inventory versions don't include %s", expHead), codes.E046.Ref(inv.Type.Spec)))
	}
	// inventory has same OCFL version as declaration
	if inv.Type.Spec != ocflV {
		err := fmt.Errorf("inventory declares OCFL version %s, NAMASTE declares %s", inv.Type.Spec, ocflV)
		vldr.LogFatal(lgr, ec(err, codes.E038.Ref(ocflV)))
	}
	// add root inventory manifest to digest ledger
	if err := vldr.ledger.addInventory(inv, true); err != nil {
		// err indicates inventory includes different digest for a previously added file
		vldr.LogFatal(lgr, ec(err, codes.E066.Ref(ocflV)))
	}
	if err := vldr.Result.Err(); err != nil {
		return err
	}
	vldr.rootInv = inv
	return nil
}

func (vldr *objectValidator) validateVersion(ctx context.Context, ver ocfl.VNum) error {
	ocflV := vldr.root.Spec // assumed ocfl version (until inventory is decoded)
	lgr := vldr.opts.Logger.WithName(ver.String())
	vDir := path.Join(vldr.Root, ver.String())
	entries, err := vldr.FS.ReadDir(ctx, vDir)
	if err != nil {
		vldr.LogFatal(lgr, err)
		return err
	}
	info := newVersionDirInfo(entries)
	for _, f := range info.extraFiles {
		err := fmt.Errorf(`unexpected files in version directory: %s`, f)
		vldr.LogFatal(lgr, ec(err, codes.E015.Ref(ocflV)))
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
				vldr.LogFatal(lgr, ec(err, codes.E016.Ref(ocflV)))
			}
			continue
		}
		err := fmt.Errorf(`extra directory in %s: %s`, ver, d)
		vldr.LogWarn(lgr, ec(err, codes.W002.Ref(ocflV)))
	}
	if info.hasInventory {
		if !algorithms[info.algID] {
			err := fmt.Errorf("%w: %s", ErrDigestAlg, info.algID)
			vldr.LogFatal(lgr, ec(err, codes.E025.Ref(ocflV)))
		}
		// use default registry b/c must be sha512/256
		alg, err := digest.Get(info.algID)
		if err != nil {
			vldr.LogFatal(lgr, err)
			return err
		}
		if err := vldr.validateVersionInventory(ctx, ver, alg); err != nil {
			return err
		}
	} else {
		vldr.LogWarn(lgr, ec(errors.New("missing version inventory"), codes.W010.Ref(ocflV)))
	}
	return vldr.Err()
}

func (vldr *objectValidator) validateVersionInventory(ctx context.Context, vn ocfl.VNum, sidecarAlg digest.Alg) error {
	lgr := vldr.opts.Logger.WithName(vn.String()).WithName(inventoryFile)
	vDir := path.Join(vldr.Root, vn.String())
	name := path.Join(vDir, inventoryFile)
	alg := sidecarAlg
	opts := []ValidationOption{
		copyValidationOptions(vldr.opts),
		appendResult(vldr.Result),
		FallbackOCFL(vldr.root.Spec),
		ValidationLogger(lgr),
	}
	inv, _ := ValidateInventory(ctx, vldr.FS, name, alg, opts...)
	if err := vldr.Err(); err != nil {
		return err
	}
	// add the version inventory's OCFL version to validations state (E103)
	vldr.verSpecs[vn] = inv.Type.Spec
	if err := vldr.ledger.addInventory(inv, false); err != nil {
		// err indicates inventory reports different digest from a previous inventory
		vldr.LogFatal(lgr, ec(err, codes.E066.Ref(inv.Type.Spec)))
	}
	// Is this the HEAD version directory?
	if vn == vldr.rootInv.Head {
		if inv.digest == vldr.rootInv.digest {
			return nil // don't need to validate any further
		}
		err := fmt.Errorf("inventory in last version (%s) is not same as root inventory", vn)
		vldr.LogFatal(lgr, ec(err, codes.E064.Ref(inv.Type.Spec)))
	}
	//
	// remaining validations should check consistency between version inventory
	// and root inventory
	//
	// check expected values specified in conf
	if vldr.rootInv.ID != inv.ID {
		err := fmt.Errorf("unexpected id: %s", inv.ID)
		vldr.LogFatal(lgr, ec(err, codes.E037.Ref(inv.Type.Spec)))
	}
	if vldr.rootInv.ContentDirectory != inv.ContentDirectory {
		err := fmt.Errorf("contentDirectory is '%s', but expected '%s'", inv.ContentDirectory, vldr.rootInv.ContentDirectory)
		vldr.LogFatal(lgr, ec(err, codes.E019.Ref(inv.Type.Spec)))
	}
	if vn != inv.Head {
		err := fmt.Errorf("inventory head is %s, expected %s", inv.Head, vn)
		vldr.LogFatal(lgr, ec(err, codes.E040.Ref(inv.Type.Spec)))
	}
	// confirm that each version's logical state in the version directory
	// inventory matches the corresponding logical state in the root inventory.
	// Because the digest algorithm can change between versions, we're comparing
	// the set of logical paths, and their correspondance to
	for v, ver := range inv.Versions {
		rootVer := vldr.rootInv.Versions[v]
		rootState, err := vldr.rootInv.objectState(v.Num())
		if err != nil {
			vldr.LogFatal(lgr, err)
			return err
		}
		verState, err := inv.objectState(v.Num())
		if err != nil {
			vldr.LogFatal(lgr, err)
			return err
		}
		if !sameObjectVersionState(rootState, verState) {
			err := fmt.Errorf("logical state for version %d in root inventory doesn't match that in %s/%s", v.Num(), vn, inventoryFile)
			vldr.LogFatal(lgr, ec(err, codes.E066.Ref(inv.Type.Spec)))
		}
		if ver.Message != rootVer.Message {
			err := fmt.Errorf(`message for version %s differs from root inventory`, v)
			vldr.LogWarn(lgr, ec(err, codes.W011.Ref(inv.Type.Spec)))
		}
		if !reflect.DeepEqual(ver.User, rootVer.User) {
			err := fmt.Errorf(`user information for version %s differs from root inventory`, v)
			vldr.LogWarn(lgr, ec(err, codes.W011.Ref(inv.Type.Spec)))
		}
		if ver.Created != rootVer.Created {
			err := fmt.Errorf(`timestamp for version %s differs from root inventory`, v)
			vldr.LogWarn(lgr, ec(err, codes.W011.Ref(inv.Type.Spec)))
		}
	}
	return vldr.Err()
}

func (vldr *objectValidator) validateExtensionsDir(ctx context.Context) error {
	lgr := vldr.opts.Logger
	extDir := path.Join(vldr.Root, extensionsDir)
	items, err := vldr.FS.ReadDir(ctx, extDir)
	// log := vldr.Log.WithName(extensionsDir)
	ocflV := vldr.root.Spec
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
			vldr.LogFatal(lgr, ec(err, codes.E067.Ref(ocflV)))
			continue
		}
		_, err := extensions.Get(i.Name())
		if err != nil {
			// unknow extension
			vldr.LogWarn(lgr, ec(fmt.Errorf("%w: %s", err, i.Name()), codes.W013.Ref(ocflV)))
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
	ocflV := vldr.root.Spec
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
					vldr.LogFatal(lgr, ec(err, codes.E092.Ref(ocflV)))
				}
				if f.InFixity() {
					err := fmt.Errorf("path referenced in %s inventory fixity does not exist: %s", locStr, p)
					vldr.LogFatal(lgr, ec(err, codes.E093.Ref(ocflV)))
				}
			}
		}
		for v := range vldr.ledger.inventories {
			if v.Num() >= pVer.Num() {
				if !pInfo.referencedIn(v, inManifest) {
					err := fmt.Errorf("path not referenecd in %s manifest as expected: %s", v, p)
					vldr.LogFatal(lgr, ec(err, codes.E023.Ref(ocflV)))
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
	registry := vldr.opts.AlgRegistry
	digestSetup := func(add func(name string, algs ...digest.Alg) error) error {
		for name, pInfo := range vldr.ledger.paths {
			algs := make([]digest.Alg, 0, len(pInfo.digests))
			for id := range pInfo.digests {
				alg, err := registry.Get(id)
				if err != nil {
					return err
				}
				algs = append(algs, alg)
			}
			if len(algs) == 0 {
				// no digests associate with the path
				err := fmt.Errorf("path not referenecd in manifest as expected: %s", name)
				return ec(err, codes.E023.Ref(ocflV))
			}
			if err := add(path.Join(vldr.Root, name), algs...); err != nil {
				return fmt.Errorf("checksum interupted near: %s: %w", name, err)
			}
		}
		return nil
	}
	digestCallback := func(name string, result digest.Set, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				err = ec(err, codes.E092.Ref(ocflV))
			}
			vldr.LogFatal(lgr, err)
			return err
		}
		for alg, sum := range result {
			// convert path back from FS-relative to object-relative path
			objPath := strings.TrimPrefix(name, vldr.Root+"/")
			entry, exists := vldr.ledger.getDigest(objPath, alg)
			if !exists {
				panic(`BUG: path/algorithm not a valid key as expected`)
			}
			if !strings.EqualFold(sum, entry.digest) {
				err := &ContentDigestErr{
					Path:   name,
					Alg:    alg,
					Entry:  *entry,
					Digest: sum,
				}
				for _, l := range entry.locs {
					if l.InManifest() {
						vldr.LogFatal(lgr, ec(err, codes.E092.Ref(ocflV)))
					} else {
						vldr.LogFatal(lgr, ec(err, codes.E093.Ref(ocflV)))
					}
				}
				return err
			}
		}
		return nil
	}
	digestOpen := func(name string) (io.Reader, error) {
		return vldr.FS.OpenFile(ctx, name)
	}
	err := checksum.Run(ctx, digestSetup, digestCallback, checksum.WithOpenFunc(digestOpen))
	if err != nil {
		vldr.LogFatal(lgr, err)
		return err
	}
	return nil
}

func (vldr *objectValidator) walkVersionContent(ctx context.Context, ver ocfl.VNum) (int, error) {
	contDir := path.Join(vldr.Root, ver.String(), vldr.rootInv.ContentDirectory)
	var added int
	err := ocfl.EachFile(ctx, vldr.FS, contDir, func(name string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.Type().IsRegular() {
			// convert fs-relative path to object-relative path
			objPath := strings.TrimPrefix(name, vldr.Root+"/")
			vldr.ledger.addPathExists(objPath, ver)
			added++
		}
		return nil
	})
	return added, err
}

type versionDirInfo struct {
	hasInventory bool
	algID        string
	extraFiles   []string
	dirs         []string
}

func newVersionDirInfo(entries []fs.DirEntry) versionDirInfo {
	var info versionDirInfo
	for _, e := range entries {
		if e.Type().IsRegular() {
			if e.Name() == inventoryFile {
				info.hasInventory = true
				continue
			}
			if info.algID == "" && strings.HasPrefix(e.Name(), inventoryFile+".") {
				info.algID = strings.TrimPrefix(e.Name(), inventoryFile+".")
				continue
			}
			info.extraFiles = append(info.extraFiles, e.Name())
			continue
		}
		info.dirs = append(info.dirs, e.Name())
	}
	return info
}

// SameObjectVersionState is used to test whether two objects states are the
// same (in the context of the same object). Both objects States must share the
// the same FS and Root. It returns true if both states have the same logical
// paths corresponding to the same content paths.
func sameObjectVersionState(stateA, stateB *ocfl.ObjectState) bool {
	if stateA.FS != nil && stateB.FS != nil && stateA.FS != stateB.FS {
		return false
	}
	if stateA.Root != "" && stateB.Root != "" && stateA.Root != stateB.Root {
		return false
	}
	err := stateA.EachPath(func(name string, d string) error {
		otherDigest := stateB.Map.GetDigest(name)
		if otherDigest == "" {
			return errors.New("not the same")
		}
		contentPaths := stateA.Manifest.DigestPaths(d)
		otherPaths := stateB.Manifest.DigestPaths(otherDigest)
		if len(contentPaths) != len(otherPaths) {
			return errors.New("not the same")
		}
		sort.Strings(contentPaths)
		sort.Strings(otherPaths)
		for i, p := range contentPaths {
			if otherPaths[i] != p {
				return errors.New("not the same")
			}
		}
		return nil
	})
	if err != nil {
		return false
	}
	// make sure all logical paths in other state are also in state
	err = stateB.EachPath(func(otherName string, _ string) error {
		if stateA.GetDigest(otherName) == "" {
			return errors.New("not the same")
		}
		return nil
	})
	return err == nil
}
