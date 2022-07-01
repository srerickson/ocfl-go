package ocflv1

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl/extensions"
	"github.com/srerickson/ocfl/namaste"
	"github.com/srerickson/ocfl/object"
	"github.com/srerickson/ocfl/spec"
	"github.com/srerickson/ocfl/store"
)

type StoreLayout struct {
	Description *string `json:"description"`
	Extension   *string `json:"extension"`
}

// Store represents an existing OCFL v1.x Storage Root. It supports read-only
// access.
type Store struct {
	fsys        fs.FS
	rootDir     string // storage root
	ocflVersion spec.Num
	getPath     extensions.LayoutFunc
	layout      *StoreLayout
}

func GetStore(ctx context.Context, fsys fs.FS, root string) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dirList, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, err
	}
	decl, err := namaste.FindDelcaration(dirList)
	if err != nil {
		err := fmt.Errorf("not an ocfl storage root: %w", err)
		return nil, err
	}
	if decl.Type != namaste.StoreType {
		err := fmt.Errorf("not an ocfl storage root: %s", root)
		return nil, err
	}
	ocflVer := decl.Version
	if !ocflVerSupported[ocflVer] {
		return nil, fmt.Errorf("%s: %w", ocflVer, object.ErrOCFLVersion)
	}
	err = namaste.Validate(ctx, fsys, path.Join(root, decl.Name()))
	if err != nil {
		return nil, err
	}
	str := &Store{
		fsys:        fsys,
		rootDir:     root,
		ocflVersion: ocflVer,
	}
	for _, inf := range dirList {
		if inf.Type().IsRegular() && inf.Name() == layoutName {
			str.layout = &StoreLayout{}
			err = store.ReadLayout(fsys, root, str.layout)
			if err != nil {
				return nil, err
			}
			if str.layout.Extension != nil && *str.layout.Extension != "" {
				str.getPath, err = store.ReadLayoutFunc(fsys, root, *str.layout.Extension)
				if err != nil {
					return nil, err
				}
			}
			break
		}
	}
	if str.getPath == nil {
		//log.Printf("storage root has no configured layout, using default: %s",
		// store.DefaultLayout().Name())
		str.getPath, err = store.DefaultLayout().NewFunc()
		if err != nil {
			return nil, err
		}
	}
	return str, nil
}

func (s *Store) ScanObjects(ctx context.Context) (map[string]spec.Num, error) {
	return store.ScanObjects(ctx, s.fsys, s.rootDir)
}

// Validate performs complete validation on the store
func (s *Store) Validate(ctx context.Context, config *ValidateStoreConf) error {
	return ValidateStore(ctx, s.fsys, s.rootDir, config)
}

// func InitStore(ctx context.Context, fsys backend.Interface, root string, opts ...store.Option) (*Store, error) {
// 	conf := &store.Config{
// 		OCFLVersion: spec.MustParse("1.0"),
// 	}
// 	for _, o := range opts {
// 		o(conf)
// 	}
// 	errWrap := func(err error) error {
// 		return fmt.Errorf("initializing storage root: %w", err)
// 	}
// 	if !ocflVerSupported[conf.OCFLVersion] {
// 		return nil, fmt.Errorf("unsupported OCFL Version: %s", conf.OCFLVersion)
// 	}
// 	err := namaste.Declaration{
// 		Type:    namaste.StoreType,
// 		Version: conf.OCFLVersion}.Write(fsys, root)
// 	if err != nil {
// 		return nil, errWrap(err)
// 	}
// 	extension := conf.LayoutFallback().Name()
// 	err = store.WriteLayout(fsys, root, &StoreLayout{
// 		Description: &conf.Description,
// 		Extension:   &extension,
// 	})
// 	if err != nil {
// 		return nil, errWrap(err)
// 	}
// 	store, err := GetStore(ctx, fsys, root)
// 	if err != nil {
// 		return nil, errWrap(err)
// 	}
// 	return store, nil
// }

func (str *Store) GetPath(ctx context.Context, p string) (*Object, error) {
	return GetObject(ctx, str.fsys, path.Join(str.rootDir, p))
}

// get returns the ocfl v1 object with id.
func (str *Store) Get(ctx context.Context, id string) (*Object, error) {
	if str.getPath == nil {
		return nil, errors.New("storage root layout not set")
	}
	pth, err := str.getPath(id)
	if err != nil {
		return nil, err
	}
	return GetObject(ctx, str.fsys, path.Join(str.rootDir, pth))
}

func (s *Store) Description() string {
	if s.layout == nil || s.layout.Description == nil {
		return ""
	}
	return *s.layout.Description
}

// WIP: committing
// func (s *Store) commit(ctx context.Context, stage *store.Stage) error {
// 	writable, ok := s.fsys.(backend.Interface)
// 	if !ok {
// 		return fmt.Errorf("store fs is not writable")
// 	}
// 	if stage.Err() != nil {
// 		return fmt.Errorf("commit: %w", stage.Err())
// 	}
// 	if s.fsys != stage.Backend() {
// 		return errors.New("commit: Stage and Object Store don't share the same backend")
// 	}
// 	if stage.User.Name == "" {
// 		return errors.New("commit: name is required")
// 	}
// 	if err := ctx.Err(); err != nil {
// 		return err
// 	}
// 	// TODO lock
// 	objectRoot, err := s.getPath(stage.ObjectID())
// 	if err != nil {
// 		return fmt.Errorf("commit: %w", err)
// 	}
// 	objectRoot = path.Join(s.rootDir, objectRoot)
// 	var prevInv *Inventory
// 	obj, err := s.Get(ctx, stage.ObjectID())
// 	if err != nil {
// 		if !errors.Is(err, fs.ErrNotExist) {
// 			return fmt.Errorf("commit: %w", err)
// 		}
// 		if stage.VersionNum().Num() != 1 {
// 			return fmt.Errorf("commit: %w", err)
// 		}

// 		err := namaste.Declaration{
// 			Type:    "ocfl_object",
// 			Version: s.ocflVersion,
// 		}.Write(writable, objectRoot)
// 		if err != nil {
// 			return fmt.Errorf("commit: %w", err)
// 		}
// 	}
// 	if obj != nil {
// 		prevInv, err = obj.Inventory(ctx)
// 		if err != nil {
// 			return fmt.Errorf("commit: %w", err)
// 		}
// 	}
// 	// new inventory
// 	newInv, err := nextVersionInventory(prevInv, stage)
// 	if err != nil {
// 		return fmt.Errorf("commit: %w", err)
// 	}
// 	if err := ctx.Err(); err != nil {
// 		return err
// 	}
// 	// log.Printf("committing %s %s", stage.ObjectID(), stage.VersionNum().String())
// 	// write inventory
// 	err = object.WriteInventory(ctx, stage.Backend(), stage.StageRoot(), newInv.DigestAlgorithm, newInv)
// 	if err != nil {
// 		return fmt.Errorf("commit: %w", err)
// 	}

// 	// does backend support renaming?
// 	renamer, isRenamer := writable.(backend.Renamer)
// 	verDir := path.Join(objectRoot, stage.VersionNum().String())

// 	if stage.NewContent != nil && len(stage.NewContent.AllDigests()) > 0 {
// 		// copy new content to the object, if present
// 		srcDir := stage.ContentDir()
// 		dstDir := path.Join(verDir, newInv.ContentDirectory)
// 		if isRenamer {
// 			//log.Printf("moving stage to object root: %s -> %s", srcDir, dstDir)
// 			err = renamer.Rename(srcDir, dstDir)
// 			if err != nil {
// 				return fmt.Errorf("commit: %w", err)
// 			}
// 		} else {
// 			//log.Printf("copying stage to object root: %s -> %s", srcDir, dstDir)
// 			// use copy: walk the stage content folder and copy each file
// 			walk := func(src string, info fs.DirEntry, err error) error {
// 				if err != nil {
// 					return err
// 				}
// 				if err := ctx.Err(); err != nil {
// 					return err
// 				}
// 				if !info.IsDir() {
// 					// content path relative to stage's content directory
// 					relPath := strings.TrimPrefix(src, srcDir+"/")
// 					dst := path.Join(dstDir, relPath)
// 					err := writable.Copy(dst, src)
// 					if err != nil {
// 						return err
// 					}
// 				}
// 				return nil
// 			}
// 			err = fs.WalkDir(s.fsys, srcDir, walk)
// 			if err != nil {
// 				return fmt.Errorf("commit: %w", err)
// 			}
// 		}
// 	}

// 	// new root inventory and sidecar (side)
// 	//log.Printf("creating new object inventories")
// 	side := inventoryFile + "." + stage.DigestAlgorithm().ID()
// 	// path to stage's copy of the inventory
// 	invSrc := path.Join(stage.StageRoot(), inventoryFile)
// 	// path to new object version's copy of the inventory
// 	invVerDst := path.Join(verDir, inventoryFile)
// 	// path to stage's copy of the sidecar file
// 	sidecarSrc := path.Join(stage.StageRoot(), side)
// 	// path to new object version's copy of the sidecar file
// 	sidecarVerDst := path.Join(verDir, side)
// 	if isRenamer {
// 		err = renamer.Rename(invSrc, invVerDst)
// 		if err != nil {
// 			return fmt.Errorf("commit: %w", err)
// 		}
// 		err = renamer.Rename(sidecarSrc, sidecarVerDst)
// 		if err != nil {
// 			return fmt.Errorf("commit: %w", err)
// 		}
// 	} else {
// 		err = writable.Copy(invVerDst, invSrc)
// 		if err != nil {
// 			return fmt.Errorf("commit: %w", err)
// 		}
// 		err = writable.Copy(sidecarVerDst, sidecarSrc)
// 		if err != nil {
// 			return fmt.Errorf("commit: %w", err)
// 		}

// 	}

// 	// copy object's version inventory to object root
// 	invRootDst := path.Join(objectRoot, inventoryFile)
// 	err = writable.Copy(invRootDst, invVerDst)
// 	if err != nil {
// 		return fmt.Errorf("commit: %w", err)
// 	}
// 	// copy object's version sidecar to object root
// 	sidecarRootDst := path.Join(objectRoot, side)
// 	err = writable.Copy(sidecarRootDst, sidecarVerDst)
// 	if err != nil {
// 		return fmt.Errorf("commit: %w", err)
// 	}

// 	// if previous inventory has a different digest algorithm
// 	// we need to remove the old sidedar file
// 	if prevInv != nil && prevInv.DigestAlgorithm != stage.DigestAlgorithm() {
// 		oldSide := inventoryFile + "." + prevInv.DigestAlgorithm.ID()
// 		err = writable.RemoveAll(path.Join(objectRoot, oldSide))
// 		if err != nil {
// 			return fmt.Errorf("commit: %w", err)
// 		}
// 	}

// 	//remove stage
// 	//log.Printf("removing stage root: %s", stage.StageRoot())
// 	if err = writable.RemoveAll(stage.StageRoot()); err != nil {
// 		err = fmt.Errorf("commit succeded but failed to clear staged files: %w", err)
// 		return err
// 	}

// 	return nil
// }
