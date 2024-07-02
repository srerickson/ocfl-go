package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/extension"
	"github.com/srerickson/ocfl-go/internal/walkdirs"
	"github.com/srerickson/ocfl-go/ocflv1/codes"
	"github.com/srerickson/ocfl-go/validation"
)

func ValidateStore(ctx context.Context, fsys ocfl.FS, root string, vops ...ValidationOption) *validation.Result {
	opts, result := validationSetup(vops)
	lgr := opts.Logger
	inf, err := fsys.ReadDir(ctx, root)
	if err != nil {
		return result.LogFatal(lgr, err)
	}
	//E069: An OCFL Storage Root MUST contain a Root Conformance Declaration
	//identifying it as such.
	//E076: [The OCFL version declaration] MUST be a file in the base
	//directory of the OCFL Storage Root giving the OCFL version in the
	//filename.
	decl, err := ocfl.FindNamaste(inf)
	if err != nil {
		err := fmt.Errorf("not an ocfl storage root: %w", err)
		return result.LogFatal(lgr, ec(err, codes.E076.Ref(ocflv1_0)))
	}
	if decl.Type != storeRoot {
		err := fmt.Errorf("not an ocfl storage root: %s", root)
		return result.LogFatal(lgr, ec(err, codes.E069.Ref(ocflv1_0)))
	}
	ocflV := decl.Version
	// if !ocflVerSupported[*ocflVer] {
	// 	return nil, fmt.Errorf("%s: %w", *ocflVer, ErrOCFLVersion)
	// }

	//E075: The OCFL version declaration MUST be formatted according to the
	//NAMASTE specification.
	//E080: The text contents of [the OCFL version declaration file] MUST be
	//the same as dvalue, followed by a newline (\n).
	err = ocfl.ValidateNamaste(ctx, fsys, path.Join(root, decl.Name()))
	if err != nil {
		result.LogFatal(lgr, ec(err, codes.E080.Ref(ocflV)))
	}

	var hasExtensions, hasLayout bool
	for _, entry := range inf {
		if entry.IsDir() && entry.Name() == extensionsDir {
			hasExtensions = true
			continue
		}
		if entry.Type().IsRegular() && entry.Name() == layoutName {
			hasLayout = true
		}
	}
	//E067: The extensions directory must not contain any files, and no sub-directories other than extension sub-directories.
	if hasExtensions {
		entries, err := fsys.ReadDir(ctx, path.Join(root, extensionsDir))
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return result.LogFatal(lgr, err)
			}
		}
		for _, e := range entries {
			if !e.IsDir() {
				err := fmt.Errorf("unexpected file in extensions directory: %s", e.Name())
				return result.LogFatal(lgr, ec(err, codes.E067.Ref(ocflV)))
			}
		}
	}

	//E068: The specific structure and function of the extension, as well as
	//a declaration of the registered extension name must be defined in one of
	//the following locations: The OCFL Extensions repository OR The Storage
	//Root, as a plain text document directly in the Storage Root.
	//- Not clear how to implement this.
	//E070: If present, [the ocfl_layout.json document] MUST include the
	//following two keys in the root JSON object: [extension, description]
	//E071: The value of the [ocfl_layout.json] extension key must be the
	//registered extension name for the extension defining the arrangement under
	//the storage root.
	var layoutConfig storeConfig
	var layout extension.Layout
	if hasLayout {
		err = readStoreConfig(ctx, fsys, root, &layoutConfig)
		if err != nil {
			result.LogFatal(lgr, err)
		}
		if _, ok := layoutConfig[descriptionKey]; !ok {
			err := errors.New(`storage root ocfl_layout.json missing key: "description"`)
			result.LogFatal(lgr, ec(err, codes.E070.Ref(ocflV)))
		}
		_, ok := layoutConfig[extensionKey]
		if !ok {
			err := errors.New(`storage root ocfl_layout.json missing key: "extension"`)
			result.LogFatal(lgr, ec(err, codes.E070.Ref(ocflV)))
		} else {
			ext, err := extension.Get(layoutConfig[extensionKey])
			if err != nil {
				return result.LogFatal(lgr, ec(err, codes.E071.Ref(ocflV)))
			}
			layout, err = readLayout(ctx, fsys, root, ext.Name())
			if err != nil {
				return result.LogFatal(lgr, err)
			}
		}
	}

	//E072: The directory hierarchy used to store OCFL Objects MUST NOT
	//contain files that are not part of an OCFL Object.
	//E073: Empty directories MUST NOT appear under a storage root.
	//E081: OCFL Objects within the OCFL Storage Root also include a
	//conformance declaration which MUST indicate OCFL Object conformance to the
	//same or earlier version of the specification.
	//E084: Storage hierarchies MUST NOT include files within intermediate
	//directories.
	//E085: Storage hierarchies MUST be terminated by OCFL Object Roots.
	//E088: An OCFL Storage Root MUST NOT contain directories or
	//sub-directories other than as a directory hierarchy used to store OCFL
	//Objects or for storage root extensions.
	validateObjectRoot := func(objRoot *ocfl.ObjectRoot) error {
		objLgr := lgr
		if objLgr != nil {
			objLgr = objLgr.With("object_path", objRoot.Path)
		}
		if ocflV.Cmp(objRoot.State.Spec) < 0 {
			// object ocfl spec is higher than storage root's
			result.LogFatal(objLgr, ErrObjectVersion)
		}
		if opts.SkipObjects {
			return nil
		}
		obj := &Object{ObjectRoot: objRoot}
		objValidOpts := []ValidationOption{
			copyValidationOptions(opts),
			ValidationLogger(objLgr),
			appendResult(result),
		}
		if err := obj.Validate(ctx, objValidOpts...).Err(); err != nil {
			return nil // return nil to continue validating objects in the Scan
		}
		if layout != nil {
			id := obj.Inventory.ID
			p, err := layout.Resolve(id)
			if err != nil {
				err := fmt.Errorf("object id '%s' is not compatible with the storage root layout: %w", id, err)
				result.LogWarn(objLgr, err)
				return nil
			}
			if expRoot := path.Join(root, p); expRoot != objRoot.Path {
				err := fmt.Errorf("object path '%s' does not conform with storage root layout. expected '%s'", objRoot.Path, expRoot)
				result.LogWarn(objLgr, err)
				return nil
			}
		}
		return nil
	}
	walkDirsFn := func(name string, entries []fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		decl, _ := ocfl.FindNamaste(entries)
		switch decl.Type {
		case ocfl.NamasteTypeObject:
			objRoot := &ocfl.ObjectRoot{
				FS:    fsys,
				Path:  name,
				State: ocfl.ParseObjectRootDir(entries),
			}
			validateObjectRoot(objRoot)
			return walkdirs.ErrSkipDirs // don't continue scan further into the object
		case ocfl.NamasteTypeStore:
			// store within a store is an error
			if name != root {
				err := fmt.Errorf("%w: %s", ErrNonObject, name)
				result.LogFatal(lgr, ec(err, codes.E084.Ref(ocflV)))
			}
		default:
			// directories without a declaration must include sub-directories
			// and only sub-directories -- however, the extensions directory
			// may be empty. we shouldn't be walking extensions dir
			numfiles := 0
			for _, e := range entries {
				if e.Type().IsRegular() {
					numfiles++
				}
			}
			if len(entries) == 0 {
				err := fmt.Errorf("%w: %s", ErrEmptyDirs, name)
				result.LogFatal(lgr, ec(err, codes.E073.Ref(ocflV)))
			}
			if numfiles > 0 {
				err := fmt.Errorf("%w: %s", ErrNonObject, name)
				result.LogFatal(lgr, ec(err, codes.E084.Ref(ocflV)))
			}
		}
		return nil
	}

	skip := func(name string) bool {
		return name == path.Join(root, extensionsDir)
	}
	if err := walkdirs.WalkDirs(ctx, fsys, root, skip, walkDirsFn, 0); err != nil {
		result.LogFatal(lgr, err)
	}
	return result
}
