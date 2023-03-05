package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/extensions"
	"github.com/srerickson/ocfl/ocflv1/codes"
	"github.com/srerickson/ocfl/validation"
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
	decl, err := ocfl.FindDeclaration(inf)
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
	err = ocfl.ValidateDeclaration(ctx, fsys, path.Join(root, decl.Name()))
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
	var layout storeConfig
	var layoutFunc extensions.LayoutFunc
	if hasLayout {
		err = readStoreConfig(ctx, fsys, root, &layout)
		if err != nil {
			result.LogFatal(lgr, err)
		}
		if _, ok := layout[descriptionKey]; !ok {
			err := errors.New(`storage root ocfl_layout.json missing key: "description"`)
			result.LogFatal(lgr, ec(err, codes.E070.Ref(ocflV)))
		}
		_, ok := layout[extensionKey]
		if !ok {
			err := errors.New(`storage root ocfl_layout.json missing key: "extension"`)
			result.LogFatal(lgr, ec(err, codes.E070.Ref(ocflV)))
		} else {
			ext, err := extensions.Get(layout[extensionKey])
			if err != nil {
				return result.LogFatal(lgr, ec(err, codes.E071.Ref(ocflV)))
			}
			if err := readExtensionConfig(ctx, fsys, root, ext); err != nil {
				err := fmt.Errorf("storage root has misconfigured layout extension: %w", err)
				return result.LogFatal(lgr, err)
			}
			lyt, ok := ext.(extensions.Layout)
			if !ok {
				return result.LogFatal(lgr, ec(extensions.ErrNotLayout, codes.E071.Ref(ocflV)))
			}
			layoutFunc, err = lyt.NewFunc()
			if err != nil {
				err := fmt.Errorf("storage root has misconfigured layout extension: %w", err)
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
	scanFn := func(obj *Object) error {
		objRoot := obj.rootDir
		objSpec := obj.info.Declaration.Version
		objLgr := lgr.WithName(objRoot)
		if ocflV.Cmp(objSpec) < 0 {
			// object ocfl spec is higher than storage root's
			result.LogFatal(objLgr, ErrObjectVersion)
		}
		if opts.SkipObjects {
			return nil
		}
		errMsg := "invalid object"
		//objPath := path.Join(root, objRoot)
		objValidOpts := []ValidationOption{
			copyValidationOptions(opts),
			ValidationLogger(objLgr),
			appendResult(result),
		}
		if err := obj.Validate(ctx, objValidOpts...).Err(); err != nil {
			return nil // return nil to continue validating objects in the Scan
		}
		inv, err := obj.Inventory(ctx) // I just need the ID
		if err != nil {
			result.LogFatal(objLgr, fmt.Errorf("%s: %w", errMsg, err))
			return nil
		}
		if layoutFunc != nil {
			p, err := layoutFunc(inv.ID)
			if err != nil {
				err := fmt.Errorf("object id '%s' is not compatible with the storage root layout: %w", inv.ID, err)
				result.LogWarn(objLgr, err)
				return nil
			}
			if expRoot := path.Join(root, p); expRoot != objRoot {
				err := fmt.Errorf("object path '%s' does not conform with storage root layout. expected '%s'", objRoot, expRoot)
				result.LogWarn(objLgr, err)
				return nil
			}
		}
		return nil
	}
	if err := ScanObjects(ctx, fsys, root, scanFn, &ScanObjectsOpts{Strict: true}); err != nil {
		if errors.Is(err, ErrEmptyDirs) {
			result.LogFatal(lgr, ec(err, codes.E073.Ref(ocflV)))
		}
		if errors.Is(err, ErrNonObject) {
			result.LogFatal(lgr, ec(err, codes.E084.Ref(ocflV)))
		}
	}
	return result
}
