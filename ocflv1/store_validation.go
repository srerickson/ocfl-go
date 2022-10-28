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

type ValidateStoreConf struct {
	validation.Log
	SkipObjects bool // don't validate objects in the store
	SkipDigests bool // don't validate object digiests
}

func ValidateStore(ctx context.Context, fsys ocfl.FS, root string, config *ValidateStoreConf) error {
	vldr := storeValidator{
		FS:   fsys,
		Root: root,
	}
	if config != nil {
		vldr.ValidateStoreConf = *config
	}
	return vldr.validate(ctx)
}

type storeValidator struct {
	ValidateStoreConf
	FS         ocfl.FS
	Root       string
	ocflV      ocfl.Spec
	Layout     storeLayout
	layoutFunc extensions.LayoutFunc
}

func (s *storeValidator) validate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	inf, err := s.FS.ReadDir(ctx, s.Root)
	if err != nil {
		return s.AddFatal(err)
	}

	//E069: An OCFL Storage Root MUST contain a Root Conformance Declaration
	//identifying it as such.
	//E076: [The OCFL version declaration] MUST be a file in the base
	//directory of the OCFL Storage Root giving the OCFL version in the
	//filename.
	decl, err := ocfl.FindDeclaration(inf)
	if err != nil {
		err := fmt.Errorf("not an ocfl storage root: %w", err)
		return s.AddFatal(ec(err, codes.E076.Ref(ocflv1_0)))
	}
	if decl.Type != storeRoot {
		err := fmt.Errorf("not an ocfl storage root: %s", s.Root)
		return s.AddFatal(ec(err, codes.E069.Ref(ocflv1_0)))
	}
	s.ocflV = decl.Version
	// if !ocflVerSupported[*ocflVer] {
	// 	return nil, fmt.Errorf("%s: %w", *ocflVer, ErrOCFLVersion)
	// }

	//E075: The OCFL version declaration MUST be formatted according to the
	//NAMASTE specification.
	//E080: The text contents of [the OCFL version declaration file] MUST be
	//the same as dvalue, followed by a newline (\n).
	err = ocfl.ValidateDeclaration(ctx, s.FS, path.Join(s.Root, decl.Name()))
	if err != nil {
		return s.AddFatal(ec(err, codes.E080.Ref(s.ocflV)))
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
		entries, err := s.FS.ReadDir(ctx, path.Join(s.Root, extensionsDir))
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return s.AddFatal(err)
			}
		}
		for _, e := range entries {
			if !e.IsDir() {
				err := fmt.Errorf("unexpected file in extensions directory: %s", e.Name())
				return s.AddFatal(ec(err, codes.E067.Ref(s.ocflV)))
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
	if hasLayout {
		err = readLayout(ctx, s.FS, s.Root, &s.Layout)
		if err != nil {
			s.AddFatal(err)
		}
		if _, ok := s.Layout[descriptionKey]; !ok {
			err := errors.New(`storage root ocfl_layout.json missing key: "description"`)
			s.AddFatal(ec(err, codes.E070.Ref(s.ocflV)))
		}
		_, ok := s.Layout[extensionKey]
		if !ok {
			err := errors.New(`storage root ocfl_layout.json missing key: "extension"`)
			s.AddFatal(ec(err, codes.E070.Ref(s.ocflV)))
		} else {
			ext, err := extensions.Get(s.Layout[extensionKey])
			if err != nil {
				return s.AddFatal(ec(err, codes.E071.Ref(s.ocflV)))
			}
			if err := readExtensionConfig(ctx, s.FS, s.Root, ext); err != nil {
				err := fmt.Errorf("storage root has misconfigured layout extension: %w", err)
				return s.AddFatal(err)
			}
			lyt, ok := ext.(extensions.Layout)
			if !ok {
				return s.AddFatal(ec(extensions.ErrNotLayout, codes.E071.Ref(s.ocflV)))
			}
			s.layoutFunc, err = lyt.NewFunc()
			if err != nil {
				err := fmt.Errorf("storage root has misconfigured layout extension: %w", err)
				return s.AddFatal(err)
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
	scan, err := ScanObjects(ctx, s.FS, s.Root, &ScanObjectsOpts{
		Strict: true,
	})
	if err != nil {
		if errors.Is(err, ErrEmptyDirs) {
			s.AddFatal(ec(err, codes.E073.Ref(s.ocflV)))
		}
		if errors.Is(err, ErrNonObject) {
			s.AddFatal(ec(err, codes.E084.Ref(s.ocflV)))
		}
	}
	for p, v := range scan {
		if s.ocflV.Cmp(v) < 0 {
			err = fmt.Errorf("%w: %s", ErrObjectVersion, p)
			s.AddFatal(err)
		}
		if s.SkipObjects {
			continue
		}
		errMsg := "storage root includes an invalid object"
		objPath := path.Join(s.Root, p)
		// FIXME: the object inventory is read twice, for GetObject and then
		// again ValidateObject. The first time is just to get the ID for
		// checking the object location. The second time is for validation.
		obj, err := GetObject(ctx, s.FS, objPath)
		if err != nil {
			s.AddFatal(fmt.Errorf("%s: %w", errMsg, err))
			continue
		}
		inv, err := obj.Inventory(ctx) // I just need the ID
		if err != nil {
			s.AddFatal(fmt.Errorf("%s: %w", errMsg, err))
			continue
		}
		if s.layoutFunc != nil {
			p, err := s.layoutFunc(inv.ID)
			if err != nil {
				err := fmt.Errorf("object id '%s' is not compatible with the storage root layout: %w", inv.ID, err)
				s.AddWarn(err)
				continue
			}
			if p != objPath {
				err := fmt.Errorf("object path '%s' does not conform with storage root layout. expected '%s'", objPath, p)
				s.AddWarn(err)
			}
		}
		objVCnf := ValidateObjectConf{
			Log:         s.Log,
			SkipDigests: s.SkipDigests,
		}
		ValidateObject(ctx, s.FS, objPath, &objVCnf)
	}
	return s.Err()
}
