package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"reflect"
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
	obj := &Object{
		fsys:    fsys,
		rootDir: root,
		info:    &vldr.rootInfo,
	}
	return obj, result
}

// objectValidator represents state of an object validation process
type objectValidator struct {
	*validation.Result
	opts *validationOptions
	FS   ocfl.FS // required
	Root string  // required

	// entries belows are state set during validation
	rootInfo ocfl.ObjInfo // info from root object
	rootInv  *Inventory   // TODO: remove, state instead
	ledger   *pathLedger
	verSpecs map[ocfl.VNum]ocfl.Spec
}

// ValidateObject performs complete validation of the object at path p in fsys.
func (vldr *objectValidator) validate(ctx context.Context) *validation.Result {
	lgr := vldr.opts.Logger
	rootList, err := vldr.FS.ReadDir(ctx, vldr.Root)
	if err != nil {
		return vldr.LogFatal(lgr, err)
	}
	vldr.rootInfo = *ocfl.ObjInfoFromFS(rootList)
	if vldr.rootInfo.Declaration.Name() == "" {
		err := ec(ocfl.ErrDeclMissing, codes.E003.Ref(ocflv1_0))
		return vldr.LogFatal(lgr, err)
	}
	ocflV := vldr.rootInfo.Declaration.Version
	switch ocflV {
	case ocfl.Spec{1, 0}:
		fallthrough
	case ocfl.Spec{1, 1}:
		if err := vldr.validateRoot(ctx); err != nil {
			return vldr.Result
		}
		for _, vnum := range vldr.rootInfo.VersionDirs {
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
	ocflV := vldr.rootInfo.Declaration.Version
	lgr := vldr.opts.Logger
	vldr.validateNamaste(ctx)
	for _, name := range vldr.rootInfo.Unknown {
		err := fmt.Errorf(`%w: %s`, ErrObjRootStructure, name)
		vldr.LogFatal(lgr, ec(err, codes.E001.Ref(ocflV)))
	}
	if !vldr.rootInfo.HasInventoryFile {
		err := fmt.Errorf(`%w: not found`, ErrInventoryOpen)
		vldr.LogFatal(lgr, ec(err, codes.E063.Ref(ocflV)))
	}
	if vldr.rootInfo.Algorithm.ID() == "" { // empty algorithm indicates missing sidecar file in root
		err := fmt.Errorf(`%w: not found`, ErrInvSidecarOpen)
		vldr.LogFatal(lgr, ec(err, codes.E058.Ref(ocflV)))
	} else if !algorithms[vldr.rootInfo.Algorithm] {
		err := fmt.Errorf(`%w: %s`, ErrDigestAlg, vldr.rootInfo.Algorithm)
		vldr.LogFatal(lgr, ec(err, codes.E025.Ref(ocflV)))
	}
	err := vldr.rootInfo.VersionDirs.Valid()
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
	if err == nil && vldr.rootInfo.VersionDirs.Padding() > 0 {
		err := errors.New("version directory names are zero-padded")
		vldr.LogWarn(lgr, ec(err, codes.W001.Ref(ocflV)))
	}
	return vldr.validateRootInventory(ctx)
}

func (vldr *objectValidator) validateNamaste(ctx context.Context) error {
	ocflV := vldr.rootInfo.Declaration.Version
	lgr := vldr.opts.Logger
	if vldr.rootInfo.Declaration.Type != ocfl.DeclObject {
		err := fmt.Errorf("%w: %s", ErrOCFLVersion, ocflV)
		vldr.LogFatal(lgr, ec(err, codes.E004.Ref(ocflV)))
	}
	err := ocfl.ValidateDeclaration(ctx, vldr.FS, path.Join(vldr.Root, vldr.rootInfo.Declaration.Name()))
	if err != nil {
		err = ec(err, codes.E007.Ref(ocflV))
		vldr.LogFatal(lgr, err)
	}
	return vldr.Err()
}

func (vldr *objectValidator) validateRootInventory(ctx context.Context) error {
	ocflV := vldr.rootInfo.Declaration.Version
	lgr := vldr.opts.Logger
	name := path.Join(vldr.Root, inventoryFile)
	alg := vldr.rootInfo.Algorithm
	opts := []ValidationOption{
		copyValidationOptions(vldr.opts),
		appendResult(vldr.Result),
		FallbackOCFL(vldr.rootInfo.Declaration.Version),
		ValidationLogger(lgr.WithName(inventoryFile)),
	}
	inv, _ := ValidateInventory(ctx, vldr.FS, name, alg, opts...)
	if err := vldr.Err(); err != nil {
		return err
	}
	// Inventory head/versions are consitent with Object Root
	if expHead := vldr.rootInfo.VersionDirs.Head(); expHead != inv.Head {
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
	ocflV := vldr.rootInfo.Declaration.Version // assumed ocfl version (until inventory is decoded)
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
		if !algorithms[info.digestAlgorithm] {
			err := fmt.Errorf("%w: %s", ErrDigestAlg, info.digestAlgorithm)
			vldr.LogFatal(lgr, ec(err, codes.E025.Ref(ocflV)))
		}
		if err := vldr.validateVersionInventory(ctx, ver, info.digestAlgorithm); err != nil {
			return err
		}
	} else {
		vldr.LogWarn(lgr, ec(errors.New("missing version inventory"), codes.W010.Ref(ocflV)))
	}
	return vldr.Err()
}

func (vldr *objectValidator) validateVersionInventory(ctx context.Context, ver ocfl.VNum, sidecarAlg digest.Alg) error {
	lgr := vldr.opts.Logger.WithName(ver.String()).WithName(inventoryFile)
	vDir := path.Join(vldr.Root, ver.String())
	name := path.Join(vDir, inventoryFile)
	alg := sidecarAlg
	opts := []ValidationOption{
		copyValidationOptions(vldr.opts),
		appendResult(vldr.Result),
		FallbackOCFL(vldr.rootInfo.Declaration.Version),
		ValidationLogger(lgr),
	}
	inv, _ := ValidateInventory(ctx, vldr.FS, name, alg, opts...)
	if err := vldr.Err(); err != nil {
		return err
	}

	// add the version inventory's OCFL version to validations state (E103)
	vldr.verSpecs[ver] = inv.Type.Spec
	if err := vldr.ledger.addInventory(inv, false); err != nil {
		// err indicates inventory reports different digest from a previous inventory
		vldr.LogFatal(lgr, ec(err, codes.E066.Ref(inv.Type.Spec)))
	}
	//
	// head version inventory?
	//
	if ver == vldr.rootInv.Head {
		if inv.digest == vldr.rootInv.digest {
			return nil // don't need to validate any further
		}
		err := fmt.Errorf("inventory in last version (%s) is not same as root inventory", ver)
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
	if ver != inv.Head {
		err := fmt.Errorf("inventory head is %s, expected %s", inv.Head, ver)
		vldr.LogFatal(lgr, ec(err, codes.E040.Ref(inv.Type.Spec)))
	}
	// confirm version states in the version inventory match root inventory
	for v, ver := range inv.Versions {
		rootVer := vldr.rootInv.Versions[v]
		rootState, _ := vldr.rootInv.IndexFull(v, true, false)
		verState, _ := inv.IndexFull(v, true, false)
		changes, err := verState.Diff(rootState, inv.DigestAlgorithm)
		if err != nil {
			err := fmt.Errorf("unexpected err durring inventory diff: %w", err)
			vldr.LogFatal(lgr, err)
			return err
		}
		if !changes.Equal() {
			errFmt := "version %s state doesn't match root inventory: %s"
			if changes.Added.Len() > 0 {
				err := fmt.Errorf(errFmt, v, "unexpected files")
				vldr.LogFatal(lgr, ec(err, codes.E066.Ref(inv.Type.Spec)))
			}
			if changes.Removed.Len() > 0 {
				err := fmt.Errorf(errFmt, v, `missing file`)
				vldr.LogFatal(lgr, ec(err, codes.E066.Ref(inv.Type.Spec)))
			}
			if changes.Changed.Len() > 0 {
				err := fmt.Errorf(errFmt, v, `changed file content`)
				vldr.LogFatal(lgr, ec(err, codes.E066.Ref(inv.Type.Spec)))
			}
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
	ocflV := vldr.rootInfo.Declaration.Version
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
	ocflV := vldr.rootInfo.Declaration.Version
	lgr := vldr.opts.Logger
	// check paths exist are in included in manifsts as necessary
	for p, pInfo := range vldr.ledger.paths {
		pVer := pInfo.existsIn // version wheren content file is stored (or empty ocfl.Num)
		if pVer.Empty() {
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
	digestSetup := func(add checksum.AddFunc) error {
		for name, pInfo := range vldr.ledger.paths {
			algs := make([]digest.Alg, 0, len(pInfo.digests))
			for alg := range pInfo.digests {
				algs = append(algs, alg)
			}
			if len(algs) == 0 {
				// no digests associate with the path
				err := fmt.Errorf("path not referenecd in manifest as expected: %s", name)
				return ec(err, codes.E023.Ref(ocflV))
			}
			if !add(path.Join(vldr.Root, name), algs) {
				return fmt.Errorf("checksum interupted near: %s", name)
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
	err := checksum.Run(digestSetup, digestCallback, checksum.WithOpenFunc(digestOpen))
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
	hasInventory    bool
	digestAlgorithm digest.Alg
	extraFiles      []string
	dirs            []string
}

func newVersionDirInfo(entries []fs.DirEntry) versionDirInfo {
	var info versionDirInfo
	for _, e := range entries {
		if e.Type().IsRegular() {
			if e.Name() == inventoryFile {
				info.hasInventory = true
				continue
			}
			if info.digestAlgorithm.ID() == "" {
				algID := strings.TrimPrefix(e.Name(), inventoryFile+".")
				if alg, err := digest.NewAlg(algID); err == nil {
					info.digestAlgorithm = alg
					continue
				}
			}
			info.extraFiles = append(info.extraFiles, e.Name())
			continue
		}
		info.dirs = append(info.dirs, e.Name())
	}
	return info
}
