package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl/extensions"
	"github.com/srerickson/ocfl/namaste"
	"github.com/srerickson/ocfl/ocflv1/codes"
	"github.com/srerickson/ocfl/spec"
	"github.com/srerickson/ocfl/store"
	"github.com/srerickson/ocfl/validation"
)

type ValidateStoreConf struct {
	validation.Log
}

func ValidateStore(ctx context.Context, fsys fs.FS, root string, config *ValidateStoreConf) error {
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
	FS     fs.FS
	Root   string
	ocflV  spec.Num
	layout struct {
		Description *string `json:"description"`
		Extension   *string `json:"extension"`
	}
	getPath extensions.LayoutFunc
}

func (s *storeValidator) validate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	inf, err := fs.ReadDir(s.FS, s.Root)
	if err != nil {
		return s.AddFatal(err)
	}

	//E069: An OCFL Storage Root MUST contain a Root Conformance Declaration
	//identifying it as such.
	//E076: [The OCFL version declaration] MUST be a file in the base
	//directory of the OCFL Storage Root giving the OCFL version in the
	//filename.
	decl, err := namaste.FindDelcaration(inf)
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
	err = namaste.Validate(ctx, s.FS, path.Join(s.Root, decl.Name()))
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
		entries, err := fs.ReadDir(s.FS, path.Join(s.Root, extensionsDir))
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
		err = store.ReadLayout(s.FS, s.Root, &s.layout)
		if err != nil {
			s.AddFatal(err)
		}
		if s.layout.Description == nil {
			err := errors.New(`storage root ocfl_layout.json missing key: "description"`)
			s.AddFatal(ec(err, codes.E070.Ref(s.ocflV)))
		}
		if s.layout.Extension == nil {
			err := errors.New(`storage root ocfl_layout.json missing  key:"extension"`)
			s.AddFatal(ec(err, codes.E070.Ref(s.ocflV)))
		} else {
			s.getPath, err = store.ReadLayoutFunc(s.FS, s.Root, *s.layout.Extension)
			if err != nil {
				return s.AddFatal(ec(err, codes.E071.Ref(s.ocflV)))
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
	}
	return s.Err()
}
