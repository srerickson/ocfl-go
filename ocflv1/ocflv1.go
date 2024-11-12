// Package [ocflv1] provides an implementation of OCFL v1.0 and v1.1.
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
	"github.com/srerickson/ocfl-go/digest"
	"github.com/srerickson/ocfl-go/extension"
	"github.com/srerickson/ocfl-go/logging"
	"github.com/srerickson/ocfl-go/ocflv1/codes"
	"golang.org/x/sync/errgroup"
)

const (
	// defaults
	inventoryFile = `inventory.json`
	contentDir    = `content`
	extensionsDir = "extensions"
)

func Enable() {
	ocfl.RegisterOCLF(&OCFL{spec: ocfl.Spec1_0})
	ocfl.RegisterOCLF(&OCFL{spec: ocfl.Spec1_1})
}

// Implementation of OCFL v1.x
type OCFL struct {
	spec ocfl.Spec // 1.0 or 1.1
}

func (imp OCFL) Spec() ocfl.Spec { return imp.spec }

func (imp OCFL) NewReadInventory(raw []byte) (ocfl.ReadInventory, error) {
	inv, err := NewInventory(raw)
	if err != nil {
		return nil, err
	}
	if err := inv.Validate().Err(); err != nil {
		return nil, err
	}
	return inv.Inventory(), nil
}

func (imp OCFL) NewReadObject(fsys ocfl.FS, path string, inv ocfl.ReadInventory) ocfl.ReadObject {
	concreteInv, ok := inv.(*readInventory)
	if !ok {
		panic("inventory has wrong type")
	}
	return &ReadObject{fs: fsys, path: path, inv: &concreteInv.raw}
}

// Commits creates or updates an object by adding a new object version based
// on the implementation.
func (imp OCFL) Commit(ctx context.Context, obj ocfl.ReadObject, commit *ocfl.Commit) (ocfl.ReadObject, error) {
	writeFS, ok := obj.FS().(ocfl.WriteFS)
	if !ok {
		err := errors.New("object's backing file system doesn't support write operations")
		return nil, &ocfl.CommitError{Err: err}
	}
	newInv, err := buildInventory(obj.Inventory(), commit)
	if err != nil {
		err := fmt.Errorf("building new inventory: %w", err)
		return nil, &ocfl.CommitError{Err: err}
	}
	logger := commit.Logger
	if logger == nil {
		logger = logging.DisabledLogger()
	}
	logger = logger.With("path", obj.Path(), "id", newInv.ID, "head", newInv.Head, "ocfl_spec", newInv.Type.Spec, "alg", newInv.DigestAlgorithm)
	// xfers is a subeset of the manifest with the new content to add
	xfers, err := newContentMap(newInv)
	if err != nil {
		return nil, &ocfl.CommitError{Err: err}
	}
	// check that the stage includes all the new content
	for digest := range xfers {
		if !commit.Stage.HasContent(digest) {
			// FIXME short digest
			err := fmt.Errorf("no content for digest: %s", digest)
			return nil, &ocfl.CommitError{Err: err}
		}
	}
	// file changes start here
	// 1. create or update NAMASTE object declaration
	var oldSpec ocfl.Spec
	if obj.Inventory() != nil {
		oldSpec = obj.Inventory().Spec()
	}
	newSpec := newInv.Type.Spec
	switch {
	case ocfl.ObjectExists(obj) && oldSpec != newSpec:
		oldDecl := ocfl.Namaste{Type: ocfl.NamasteTypeObject, Version: oldSpec}
		logger.DebugContext(ctx, "deleting previous OCFL object declaration", "name", oldDecl)
		if err = writeFS.Remove(ctx, path.Join(obj.Path(), oldDecl.Name())); err != nil {
			return nil, &ocfl.CommitError{Err: err, Dirty: true}
		}
		fallthrough
	case !ocfl.ObjectExists(obj):
		newDecl := ocfl.Namaste{Type: ocfl.NamasteTypeObject, Version: newSpec}
		logger.DebugContext(ctx, "writing new OCFL object declaration", "name", newDecl)
		if err = ocfl.WriteDeclaration(ctx, writeFS, obj.Path(), newDecl); err != nil {
			return nil, &ocfl.CommitError{Err: err, Dirty: true}
		}
	}
	// 2. tranfser files from stage to object
	if len(xfers) > 0 {
		copyOpts := &copyContentOpts{
			Source:   commit.Stage,
			DestFS:   writeFS,
			DestRoot: obj.Path(),
			Manifest: xfers,
		}
		logger.DebugContext(ctx, "copying new object files", "count", len(xfers))
		if err := copyContent(ctx, copyOpts); err != nil {
			err = fmt.Errorf("transferring new object contents: %w", err)
			return nil, &ocfl.CommitError{Err: err, Dirty: true}
		}
	}
	logger.DebugContext(ctx, "writing inventories for new object version")
	// 3. write inventory to both object root and version directory
	newVersionDir := path.Join(obj.Path(), newInv.Head.String())
	if err := writeInventory(ctx, writeFS, newInv, obj.Path(), newVersionDir); err != nil {
		err = fmt.Errorf("writing new inventories or inventory sidecars: %w", err)
		return nil, &ocfl.CommitError{Err: err, Dirty: true}
	}
	return &ReadObject{
		inv:  newInv,
		fs:   obj.FS(),
		path: obj.Path(),
	}, nil
}

func (imp OCFL) ValidateObjectRoot(ctx context.Context, fsys ocfl.FS, dir string, state *ocfl.ObjectState, vldr *ocfl.ObjectValidation) (ocfl.ReadObject, error) {
	// validate namaste
	decl := ocfl.Namaste{Type: ocfl.NamasteTypeObject, Version: imp.spec}
	name := path.Join(dir, decl.Name())
	err := ocfl.ValidateNamaste(ctx, fsys, name)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			err = fmt.Errorf("%s: %w", name, ocfl.ErrObjectNamasteNotExist)
			vldr.AddFatal(ec(err, codes.E001(imp.spec)))
		default:
			vldr.AddFatal(ec(err, codes.E007(imp.spec)))
		}
		return nil, err
	}
	// validate root inventory
	invBytes, err := ocfl.ReadAll(ctx, fsys, path.Join(dir, inventoryFile))
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			vldr.AddFatal(err, ec(err, codes.E063(imp.spec)))
		default:
			vldr.AddFatal(err)
		}
		return nil, err
	}
	inv, invValidation := ValidateInventoryBytes(invBytes, imp.spec)
	vldr.PrefixAdd("root inventory.json", invValidation)
	if err := invValidation.Err(); err != nil {
		return nil, err
	}
	if err := ocfl.ValidateInventorySidecar(ctx, inv.Inventory(), fsys, dir); err != nil {
		switch {
		case errors.Is(err, ocfl.ErrInventorySidecarContents):
			vldr.AddFatal(ec(err, codes.E061(imp.spec)))
		default:
			vldr.AddFatal(ec(err, codes.E060(imp.spec)))
		}
	}
	vldr.PrefixAdd("extensions directory", validateExtensionsDir(ctx, imp.spec, fsys, dir))
	if err := vldr.AddInventoryDigests(inv.Inventory()); err != nil {
		vldr.AddFatal(err)
	}
	vldr.PrefixAdd("root contents", validateRootState(imp.spec, state))
	if err := vldr.Err(); err != nil {
		return nil, err
	}
	return &ReadObject{fs: fsys, path: dir, inv: inv}, nil
}

func (imp OCFL) ValidateObjectVersion(ctx context.Context, obj ocfl.ReadObject, vnum ocfl.VNum, verInv ocfl.ReadInventory, prevInv ocfl.ReadInventory, vldr *ocfl.ObjectValidation) error {
	fsys := obj.FS()
	vnumStr := vnum.String()
	fullVerDir := path.Join(obj.Path(), vnumStr) // version directory path relative to FS
	vSpec := imp.spec
	rootInv := obj.Inventory() // headInv is assumed to be valid
	vDirEntries, err := fsys.ReadDir(ctx, fullVerDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		// can't read version directory for some reason, but not because it
		// doesn't exist.
		vldr.AddFatal(err)
		return err
	}
	vdirState := parseVersionDirState(vDirEntries)
	for _, f := range vdirState.extraFiles {
		err := fmt.Errorf(`unexpected file in %s: %s`, vnum, f)
		vldr.AddFatal(ec(err, codes.E015(vSpec)))
	}
	if !vdirState.hasInventory {
		err := fmt.Errorf("missing %s/inventory.json", vnumStr)
		vldr.AddWarn(ec(err, codes.W010(vSpec)))
	}
	if verInv != nil {
		verInvValidation := verInv.Validate()
		vldr.PrefixAdd(vnumStr+"/inventory.json", verInvValidation)
		if err := ocfl.ValidateInventorySidecar(ctx, verInv, fsys, fullVerDir); err != nil {
			err := fmt.Errorf("%s/inventory.json: %w", vnumStr, err)
			switch {
			case errors.Is(err, ocfl.ErrInventorySidecarContents):
				vldr.AddFatal(ec(err, codes.E061(imp.spec)))
			default:
				vldr.AddFatal(ec(err, codes.E060(imp.spec)))
			}
		}
		if prevInv != nil && verInv.Spec().Cmp(prevInv.Spec()) < 0 {
			err := fmt.Errorf("%s/inventory.json uses an older OCFL specification than than the previous version", vnum)
			vldr.AddFatal(ec(err, codes.E103(vSpec)))
		}
		if verInv.Head() != vnum {
			err := fmt.Errorf("%s/inventory.json: 'head' does not matchs its directory", vnum)
			vldr.AddFatal(ec(err, codes.E040(vSpec)))
		}
		if verInv.Digest() != rootInv.Digest() {
			imp.compareVersionInventory(obj, vnum, verInv, vldr)
			if verInv.Digest() != rootInv.Digest() {
				if err := vldr.AddInventoryDigests(verInv); err != nil {
					err = fmt.Errorf("%s/inventory.json digests are inconsistent with other inventories: %w", vnum, err)
					vldr.AddFatal(ec(err, codes.E066(vSpec)))
				}
			}
		}
	}
	cdName := rootInv.ContentDirectory()
	for _, d := range vdirState.dirs {
		// the only directory in the version directory SHOULD be the content directory
		if d != cdName {
			err := fmt.Errorf(`extra directory in %s: %s`, vnum, d)
			vldr.AddWarn(ec(err, codes.W002(vSpec)))
			continue
		}
		// add version content files to validation state
		var added int
		fullVerContDir := path.Join(fullVerDir, cdName)
		contentFiles, filesErrFn := ocfl.WalkFiles(ctx, fsys, fullVerContDir)
		for contentFile := range contentFiles {
			// convert from path relative to version content directory to path
			// relative to the object
			vldr.AddExistingContent(path.Join(vnumStr, cdName, contentFile.Path))
			added++
		}
		if err := filesErrFn(); err != nil {
			vldr.AddFatal(err)
			return err
		}
		if added == 0 {
			// content directory exists but it's empty
			err := fmt.Errorf("content directory (%s) is empty directory", fullVerContDir)
			vldr.AddFatal(ec(err, codes.E016(vSpec)))
		}
	}
	return nil
}

func (imp OCFL) ValidateObjectContent(ctx context.Context, obj ocfl.ReadObject, v *ocfl.ObjectValidation) error {
	newVld := &ocfl.Validation{}
	for name := range v.MissingContent() {
		err := fmt.Errorf("missing content: %s", name)
		newVld.AddFatal(ec(err, codes.E092(imp.spec)))
	}
	for name := range v.UnexpectedContent() {
		err := fmt.Errorf("unexpected content: %s", name)
		newVld.AddFatal(ec(err, codes.E023(imp.spec)))
	}
	if !v.SkipDigests() {
		alg := obj.Inventory().DigestAlgorithm()
		digests := v.ExistingContentDigests(obj.FS(), obj.Path(), alg)
		numgos := v.DigestConcurrency()
		registry := v.ValidationAlgorithms()
		for err := range digests.ValidateBatch(ctx, registry, numgos) {
			var digestErr *digest.DigestError
			switch {
			case errors.As(err, &digestErr):
				newVld.AddFatal(ec(digestErr, codes.E093(imp.spec)))
			default:
				newVld.AddFatal(err)
			}
		}
	}
	v.Add(newVld)
	return newVld.Err()
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

func (imp OCFL) compareVersionInventory(obj ocfl.ReadObject, dirNum ocfl.VNum, verInv ocfl.ReadInventory, vldr *ocfl.ObjectValidation) {
	rootInv := obj.Inventory()
	vSpec := imp.spec
	if verInv.Head() == rootInv.Head() && verInv.Digest() != rootInv.Digest() {
		err := fmt.Errorf("%s/inventor.json is not the same as the root inventory: digests don't match", dirNum)
		vldr.AddFatal(ec(err, codes.E064(vSpec)))
	}
	if verInv.ID() != rootInv.ID() {
		err := fmt.Errorf("%s/inventory.json: 'id' doesn't match value in root inventory", dirNum)
		vldr.AddFatal(ec(err, codes.E037(vSpec)))
	}
	if verInv.ContentDirectory() != rootInv.ContentDirectory() {
		err := fmt.Errorf("%s/inventory.json: 'contentDirectory' doesn't match value in root inventory", dirNum)
		vldr.AddFatal(ec(err, codes.E019(vSpec)))
	}
	// check that all version blocks in the version inventory
	// match version blocks in the root inventory
	for _, v := range verInv.Head().Lineage() {
		thisVersion := verInv.Version(v.Num())
		rootVersion := rootInv.Version(v.Num())
		if rootVersion == nil {
			err := fmt.Errorf("root inventory.json has missing version: %s", v)
			vldr.AddFatal(ec(err, codes.E046(vSpec)))
			continue
		}
		thisVerState := logicalState{
			state:    thisVersion.State(),
			manifest: verInv.Manifest(),
		}
		rootVerState := logicalState{
			state:    rootVersion.State(),
			manifest: rootInv.Manifest(),
		}
		if !thisVerState.Eq(rootVerState) {
			err := fmt.Errorf("%s/inventory.json has different logical state in its %s version block than the root inventory.json", dirNum, v)
			vldr.AddFatal(ec(err, codes.E066(vSpec)))
		}
		if thisVersion.Message() != rootVersion.Message() {
			err := fmt.Errorf("%s/inventory.json has different 'message' in its %s version block than the root inventory.json", dirNum, v)
			vldr.AddWarn(ec(err, codes.W011(vSpec)))
		}
		if !reflect.DeepEqual(thisVersion.User(), rootVersion.User()) {
			err := fmt.Errorf("%s/inventory.json has different 'user' in its %s version block than the root inventory.json", dirNum, v)
			vldr.AddWarn(ec(err, codes.W011(vSpec)))
		}
		if thisVersion.Created() != rootVersion.Created() {
			err := fmt.Errorf("%s/inventory.json has different 'created' in its %s version block than the root inventory.json", dirNum, v)
			vldr.AddWarn(ec(err, codes.W011(vSpec)))
		}
	}
}

// newContentMap returns a DigestMap that is a subset of the inventory
// manifest for the digests and paths of new content
func newContentMap(inv *Inventory) (ocfl.DigestMap, error) {
	pm := ocfl.PathMap{}
	var err error
	inv.Manifest.EachPath(func(pth, dig string) bool {
		// ignore manifest entries from previous versions
		if !strings.HasPrefix(pth, inv.Head.String()+"/") {
			return true
		}
		if _, exists := pm[pth]; exists {
			err = fmt.Errorf("path duplicate in manifest: %q", pth)
			return false
		}
		pm[pth] = dig
		return true
	})
	if err != nil {
		return nil, err
	}
	return pm.DigestMapValid()
}

type copyContentOpts struct {
	Source      ocfl.ContentSource
	DestFS      ocfl.WriteFS
	DestRoot    string
	Manifest    ocfl.DigestMap
	Concurrency int
}

// transfer dst/src names in files from srcFS to dstFS
func copyContent(ctx context.Context, c *copyContentOpts) error {
	if c.Source == nil {
		return errors.New("missing countent source")
	}
	conc := c.Concurrency
	if conc < 1 {
		conc = 1
	}
	grp, ctx := errgroup.WithContext(ctx)
	grp.SetLimit(conc)
	for dig, dstNames := range c.Manifest {
		srcFS, srcPath := c.Source.GetContent(dig)
		if srcFS == nil {
			return fmt.Errorf("content source doesn't provide %q", dig)
		}
		for _, dstName := range dstNames {
			srcPath := srcPath
			dstPath := path.Join(c.DestRoot, dstName)
			grp.Go(func() error {
				return ocfl.Copy(ctx, c.DestFS, dstPath, srcFS, srcPath)
			})

		}
	}
	return grp.Wait()
}

func ec(err error, code *ocfl.ValidationCode) error {
	if code == nil {
		return err
	}
	return &ocfl.ValidationError{
		Err:            err,
		ValidationCode: *code,
	}
}

func validateRootState(ocflV ocfl.Spec, state *ocfl.ObjectState) *ocfl.Validation {
	v := &ocfl.Validation{}
	for _, name := range state.Invalid {
		err := fmt.Errorf(`%w: %s`, ErrObjRootStructure, name)
		v.AddFatal(ec(err, codes.E001(ocflV)))
	}
	if !state.HasInventory() {
		err := fmt.Errorf(`root inventory.json: %w`, fs.ErrNotExist)
		v.AddFatal(ec(err, codes.E063(ocflV)))
	}
	if !state.HasSidecar() {
		err := fmt.Errorf(`root inventory.json sidecar: %w`, fs.ErrNotExist)
		v.AddFatal(ec(err, codes.E058(ocflV)))
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
		v.AddFatal(err)
	}
	if err == nil && state.VersionDirs.Padding() > 0 {
		err := errors.New("version directory names are zero-padded")
		v.AddWarn(ec(err, codes.W001(ocflV)))
	}
	// if vdirHead := state.VersionDirs.Head().Num(); vdirHead > o.inv.Head.Num() {
	// 	err := errors.New("version directories don't reflect versions in inventory.json")
	// 	v.AddFatal(ec(err, codes.E046(ocflV)))
	// }
	return v
}

func validateExtensionsDir(ctx context.Context, ocflV ocfl.Spec, fsys ocfl.FS, objDir string) *ocfl.Validation {
	v := &ocfl.Validation{}
	extDir := path.Join(objDir, extensionsDir)
	items, err := fsys.ReadDir(ctx, extDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		v.AddFatal(err)
		return v
	}
	for _, i := range items {
		if !i.IsDir() {
			err := fmt.Errorf(`invalid file: %s`, i.Name())
			v.AddFatal(ec(err, codes.E067(ocflV)))
			continue
		}
		_, err := extension.Get(i.Name())
		if err != nil {
			// unknow extension
			err := fmt.Errorf("%w: %s", err, i.Name())
			v.AddWarn(ec(err, codes.W013(ocflV)))
		}
	}
	return v
}
