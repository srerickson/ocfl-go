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
	"github.com/srerickson/ocfl-go/logging"
	"github.com/srerickson/ocfl-go/ocflv1/codes"
	"golang.org/x/sync/errgroup"
)

const (
	// defaults
	inventoryFile = `inventory.json`
	contentDir    = `content`
	extensionsDir = "extensions"
	// layoutName          = "ocfl_layout.json"
	// storeRoot           = ocfl.NamasteTypeStore
	// descriptionKey      = `description`
	// extensionKey        = `extension`
	// extensionConfigFile = "config.json"
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
		if errors.Is(err, fs.ErrNotExist) {
			err = fmt.Errorf("%s: %w", name, ocfl.ErrObjectNamasteNotExist)
		}
		vldr.AddFatal(err)
		return nil, err
	}
	vldr.PrefixAdd("root contents", validateRootState(imp.spec, state))
	return nil, nil
}

func (imp OCFL) ValidateVersion(ctx context.Context, obj ocfl.ReadObject, dirNum ocfl.VNum, inv ocfl.ReadInventory, prev ocfl.ReadInventory, vldr *ocfl.ObjectValidation) error {
	fsys := obj.FS()
	vDir := path.Join(obj.Path(), dirNum.String())
	vSpec := imp.spec
	rootInv := obj.Inventory() // headInv is assumed to be valid
	vDirEntries, err := fsys.ReadDir(ctx, vDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		vldr.AddFatal(err)
		return err
	}
	info := parseVersionDirState(vDirEntries)
	for _, f := range info.extraFiles {
		err := fmt.Errorf(`unexpected file in %s: %s`, dirNum, f)
		vldr.AddFatal(ec(err, codes.E015(vSpec)))
	}
	if !info.hasInventory {
		// the version directory doesn't exist or it's empty
		err := fmt.Errorf("missing %s/inventory.json", dirNum.String())
		vldr.AddWarn(ec(err, codes.W010(vSpec)))
	}
	if inv != nil {
		imp.validateVersionInventory(obj, dirNum, inv, vldr)
		if prev != nil && inv.Spec().Cmp(prev.Spec()) < 0 {
			err := fmt.Errorf("%s/inventory.json uses an older OCFL specification than than the previous version", dirNum)
			vldr.AddFatal(ec(err, codes.E103(vSpec)))
		}
	}
	for _, d := range info.dirs {
		// directory SHOULD only be content directory
		if d != rootInv.ContentDirectory() {
			err := fmt.Errorf(`extra directory in %s: %s`, dirNum, d)
			vldr.AddWarn(ec(err, codes.W002(vSpec)))
			continue
		}
		// add version content directory to validation state
		var added int
		var iterErr error
		ocfl.Files(ctx, fsys, path.Join(vDir, rootInv.ContentDirectory()))(func(info ocfl.FileInfo, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			// convert fs-relative path to object-relative path
			vldr.AddExistingContent(strings.TrimPrefix(info.Path, obj.Path()+"/"))
			added++
			return true
		})
		if iterErr != nil {
			vldr.AddFatal(iterErr)
			return iterErr
		}
		if added == 0 {
			// content directory exists but it's empty
			err := fmt.Errorf("content directory (%s) contains no files", rootInv.ContentDirectory())
			vldr.AddFatal(ec(err, codes.E016(vSpec)))
		}
	}
	return nil
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

func (imp OCFL) validateVersionInventory(obj ocfl.ReadObject, dirNum ocfl.VNum, inv ocfl.ReadInventory, vldr *ocfl.ObjectValidation) {
	rootInv := obj.Inventory()
	vSpec := imp.spec
	if dirNum == rootInv.Head() {
		// inventory should match root inventory
		if inv.Digest() == rootInv.Digest() {
			return // we don't need to do anything
		}
		err := fmt.Errorf("%s/inventor.json is not the same as the root inventory: digests don't match", dirNum)
		vldr.AddFatal(ec(err, codes.E064(vSpec)))
	}
	vldr.PrefixAdd(dirNum.String()+"/inventory.json", inv.Validate())
	if inv.ID() != rootInv.ID() {
		err := fmt.Errorf("%s/inventory.json: 'id' doesn't match value in root inventory", dirNum)
		vldr.AddFatal(ec(err, codes.E037(vSpec)))
	}
	if inv.ContentDirectory() != rootInv.ContentDirectory() {
		err := fmt.Errorf("%s/inventory.json: 'contentDirectory' doesn't match value in root inventory", dirNum)
		vldr.AddFatal(ec(err, codes.E019(vSpec)))
	}
	// if prev != nil && inv.Spec().Cmp(prev.Spec()) < 0 {
	// 	err := fmt.Errorf("%s/inventory.json uses an older OCFL specification than than the previous version", dirNum)
	// 	vldr.AddFatal(ec(err, codes.E103(vSpec)))
	// }
	if inv.Head() != dirNum {
		err := fmt.Errorf("%s/inventory.json: 'head' does not matchs its directory (%s)", dirNum, dirNum)
		vldr.AddFatal(ec(err, codes.E040(vSpec)))
	}
	// err := fmt.Errorf("%s uses a lower version of the OCFL spec than %s (%s < %s)", vnum, prevVer, vnumSpec, prevSpec)
	// vldr.LogFatal(lgr, ec(err, codes.E103(ocflV)))

	// the version content directory must be the same
	// the ocfl spec must >=
	// check that all version states in prev match the corresponding
	// version state in this inventory
	for _, v := range inv.Head().Lineage() {
		thisVersion := inv.Version(v.Num())
		rootVersion := rootInv.Version(v.Num())
		thisVerState := logicalState{
			state:    thisVersion.State(),
			manifest: inv.Manifest(),
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
	if err := vldr.AddInventoryDigests(inv); err != nil {
		err = fmt.Errorf("%s/inventory.json digests are inconsistent with other inventories: %w", dirNum, err)
		vldr.AddFatal(ec(err, codes.E066(vSpec)))
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
