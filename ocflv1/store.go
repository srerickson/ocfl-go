package ocflv1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/go-logr/logr"
	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/digest"
	"github.com/srerickson/ocfl/extensions"
)

var ErrLayoutUndefined = errors.New("storage root layout is undefined")

// Store represents an existing OCFL v1.x Storage Root.
type Store struct {
	fsys       ocfl.FS
	rootDir    string // storage root directory
	config     storeLayout
	spec       ocfl.Spec
	layoutFunc extensions.LayoutFunc
	logger     logr.Logger
}

// store layout represent ocfl_layout.json file
type storeLayout map[string]string

type InitStoreConf struct {
	Spec        ocfl.Spec
	Description string
	Layout      extensions.Layout
	Extensions  []extensions.Extension
}

// InitStore initializes a new OCFL v1.x storage root on fsys at root.
func InitStore(ctx context.Context, fsys ocfl.WriteFS, root string, conf *InitStoreConf) error {
	if conf == nil {
		conf = &InitStoreConf{}
	}
	// default to ocfl v1.1
	if conf.Spec == (ocfl.Spec{}) {
		conf.Spec = ocflv1_1
	}
	if !ocflVerSupported[conf.Spec] {
		return fmt.Errorf("%s: %w", conf.Spec, ErrOCFLVersion)
	}
	// default to 0002-flat-direct layout
	if conf.Layout == nil {
		conf.Layout = extensions.NewLayoutFlatDirect()
	}
	decl := ocfl.Declaration{
		Type:    ocfl.DeclStore,
		Version: conf.Spec,
	}
	entries, err := fsys.ReadDir(ctx, root)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if len(entries) > 0 {
		return fmt.Errorf("directory '%s' is not empty", root)
	}
	if err := ocfl.WriteDeclaration(ctx, fsys, root, decl); err != nil {
		return err
	}
	layt := newLayout(conf.Description, conf.Layout)
	if err := writeLayout(ctx, fsys, root, layt); err != nil {
		return err
	}
	if err := writeExtensionConfig(ctx, fsys, root, conf.Layout); err != nil {
		return err
	}
	for _, e := range conf.Extensions {
		if err := writeExtensionConfig(ctx, fsys, root, e); err != nil {
			return err
		}
	}
	return nil
}

type GetStoreConf struct {
	Logger     logr.Logger
	SkipLayout bool
}

// GetStore returns a *Store for the OCFL Storage Root at root in fsys. The path
// root must be a directory/prefix with storage root declaration file.
func GetStore(ctx context.Context, fsys ocfl.FS, root string, conf *GetStoreConf) (*Store, error) {
	if conf == nil {
		conf = &GetStoreConf{
			Logger: logr.Discard(),
		}
	}
	dirList, err := fsys.ReadDir(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("reading storage root: %w", err)
	}
	decl, err := ocfl.FindDeclaration(dirList)
	if err != nil {
		err := fmt.Errorf("not an ocfl storage root: %w", err)
		return nil, err
	}
	if decl.Type != ocfl.DeclStore {
		err := fmt.Errorf("not an ocfl storage root: %s", root)
		return nil, err
	}
	ocflVer := decl.Version
	if !ocflVerSupported[ocflVer] {
		return nil, fmt.Errorf("%s: %w", ocflVer, ErrOCFLVersion)
	}
	err = ocfl.ValidateDeclaration(ctx, fsys, path.Join(root, decl.Name()))
	if err != nil {
		return nil, fmt.Errorf("storage root declaration is invalid: %w", err)
	}
	str := &Store{
		fsys:    fsys,
		rootDir: root,
		spec:    ocflVer,
		logger:  conf.Logger,
	}
	for _, inf := range dirList {
		if inf.Type().IsRegular() && inf.Name() == layoutName {
			str.config = storeLayout{}
			err = readLayout(ctx, fsys, root, &str.config)
			if err != nil {
				return nil, fmt.Errorf("reading storage root ")
			}
			break
		}
	}
	if str.config != nil && !conf.SkipLayout {
		if err := str.ReadLayout(ctx); err != nil {
			return nil, fmt.Errorf("failed to set store's layout: %w", err)
		}
	}
	if !str.LayoutOK() {
		str.logger.V(ocfl.LevelWarning).Info("storage root's layout is not set")
	}
	return str, nil
}

// Descriptions returns the description set in the storage root's
// ocfl_layout.json file, or an empty string if the description is undefined
func (s *Store) Description() string {
	if s.config == nil {
		return ""
	}
	return s.config[descriptionKey]
}

// LayoutName returns the extension name set in the storage root's
// ocfl_layout.json file, or an empty string if the name is not set.
func (s *Store) LayoutName() string {
	if s.config == nil {
		return ""
	}
	return s.config[extensionKey]
}

// Spec returns the ocfl.Spec defined in the storage root's declaration.
func (s *Store) Spec() ocfl.Spec {
	return s.spec
}

// ScanObjects scans the storage root for objects, returning a map of path/ocfl
// version pairs. No validation checks are performed.
func (s *Store) ScanObjects(ctx context.Context, opts *ScanObjectsOpts) (map[string]ocfl.Spec, error) {
	return ScanObjects(ctx, s.fsys, s.rootDir, opts)
}

// Validate performs complete validation on the store
func (s *Store) Validate(ctx context.Context, config *ValidateStoreConf) error {
	return ValidateStore(ctx, s.fsys, s.rootDir, config)
}

// ResolveID resolves the storage path for the given id relative using the
// storage root's layout extension
func (s *Store) ResolveID(id string) (string, error) {
	if s.layoutFunc == nil {
		return "", ErrLayoutUndefined
	}
	return s.layoutFunc(id)
}

// GetObjectPath returns an Object for the given path relative to the storage root.
func (s *Store) GetObjectPath(ctx context.Context, p string) (*Object, error) {
	return GetObject(ctx, s.fsys, path.Join(s.rootDir, p))
}

// GetObject returns the OCFL object  using the store's layout
// extension (if defined). The store layout is set during GetStore() if the
// storage root includes an ocfl_layout.json file. Otherwise, it can be set
// using, SetLayout().
func (s *Store) GetObject(ctx context.Context, id string) (*Object, error) {
	pth, err := s.ResolveID(id)
	if err != nil {
		return nil, err
	}
	return GetObject(ctx, s.fsys, path.Join(s.rootDir, pth))
}

// SetLayout sets the store's active layout. If no error is returned, subsequent
// calls to ResolveID() will resolve object ids using the new layout.
func (s *Store) SetLayout(layout extensions.Layout) error {
	getPath, err := layout.NewFunc()
	if err != nil {
		return err
	}
	s.layoutFunc = getPath
	return nil
}

// LayoutSet returns boolean indicating if the store's layout function is set
func (s *Store) LayoutOK() bool {
	return s.layoutFunc != nil
}

// ReadLayout resolves the layout extension defined in ocfl_layout.json and
// loads its configuration file (if present) from the store's extensions
// directory. The store's active layout is set to the new layout. If no error is
// returned, subsequent calls to ResolveID() will resolve object ids using the
// new layout.
func (s *Store) ReadLayout(ctx context.Context) error {
	name := s.LayoutName()
	if name == "" {
		return ErrLayoutUndefined
	}
	ext, err := s.readExtensionConfig(ctx, name)
	if err != nil {
		return err
	}
	layoutExt, ok := ext.(extensions.Layout)
	if !ok {
		return fmt.Errorf("%s: %w", name, extensions.ErrNotLayout)
	}
	return s.SetLayout(layoutExt)
}

// StageNew creates a stage for creating the first version on an object with the
// given id.
func (s *Store) StageNew(ctx context.Context, id string, opts ...StageOpt) (*Stage, error) {
	stage := &Stage{
		id:              id,
		vnum:            ocfl.V1,
		spec:            s.spec,
		alg:             digest.SHA512,
		contentDir:      contentDir,
		contentPathFunc: DefaultContentPathFunc,
		srcFS:           ocfl.NewFS(os.DirFS(".")),
		state:           &digest.Tree{},
		manifest:        digest.NewMap(),
	}
	for _, opt := range opts {
		opt(stage)
	}
	return stage, nil
}

// StageNext creates a stage for next the version of an existing object.
func (s *Store) StageNext(ctx context.Context, obj *Object, opts ...StageOpt) (*Stage, error) {
	inv, err := obj.Inventory(ctx)
	if err != nil {
		return nil, err
	}
	nextV, err := inv.Head.Next()
	if err != nil {
		return nil, err
	}
	state := &digest.Tree{}
	for p, d := range inv.Versions[inv.Head].State.AllPaths() {
		if err := state.SetDigest(p, d, false); err != nil {
			return nil, err
		}
	}
	stage := &Stage{
		id:              inv.ID,
		vnum:            nextV,
		spec:            s.spec, // ocfl version from store
		alg:             inv.DigestAlgorithm,
		contentDir:      inv.ContentDirectory,
		contentPathFunc: DefaultContentPathFunc,
		state:           state,
		manifest:        digest.NewMap(),
	}
	for _, opt := range opts {
		opt(stage)
	}
	if err := stage.validate(inv); err != nil {
		return nil, fmt.Errorf("stage options are not valid for this object: %w", err)
	}
	return stage, nil
}

// Commit creates or updates an object in the store using stage.
func (s *Store) Commit(ctx context.Context, stage *Stage) error {
	writeFS, ok := s.fsys.(ocfl.WriteFS)
	if !ok {
		return fmt.Errorf("storage root backend is read-only")
	}
	var inv *Inventory
	var invErr error
	if stage.vnum.Num() == 1 {
		inv, invErr = buildInventoryV1(ctx, stage)

	} else {
		obj, err := s.GetObject(ctx, stage.id)
		if err != nil {
			return fmt.Errorf("retrieving object: %w", err)
		}
		prev, err := obj.Inventory(ctx)
		if err != nil {
			return fmt.Errorf("retrieving object inventory: %w", err)
		}
		inv, invErr = buildInventoryNext(ctx, stage, prev)
	}
	if invErr != nil {
		return fmt.Errorf("building inventory from stage: %w", invErr)
	}
	// safe to commit?
	if s.layoutFunc == nil {
		return fmt.Errorf("storage root layout must be set to commit: %w", ErrLayoutUndefined)
	}
	objPath, err := s.layoutFunc(stage.id)
	if err != nil {
		return fmt.Errorf("object ID must be valid to commit: %w", err)
	}
	objPath = path.Join(s.rootDir, objPath)
	// expect version directory to ErrNotExist or be empty
	if stage.vnum.Num() == 1 {
		entries, err := s.fsys.ReadDir(ctx, objPath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if len(entries) != 0 {
			return errors.New("object directory must be empty to commit")
		}
	} else {
		entries, err := s.fsys.ReadDir(ctx, path.Join(objPath, stage.vnum.String()))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("version directory '%s' must be empty to commit", stage.vnum.String())
		}
	}
	// copy files from srcFS to storage root fs
	for p, d := range inv.Manifest.AllPaths() {
		if !strings.HasPrefix(p, stage.vnum.String()) {
			continue
		}
		sources := stage.manifest.DigestPaths(d)
		if len(sources) == 0 {
			return fmt.Errorf("stage doesn't provide a source for digest: %s", d)
		}
		reader, err := stage.srcFS.OpenFile(ctx, sources[0])
		if err != nil {
			return err
		}
		defer reader.Close()
		dst := path.Join(objPath, p)
		_, err = writeFS.Write(ctx, dst, reader)
		if err != nil {
			return err
		}
	}
	// write declaration and inventory
	decl := ocfl.Declaration{
		Type:    ocfl.DeclObject,
		Version: stage.spec,
	}
	if stage.vnum.Num() == 1 {
		if err := ocfl.WriteDeclaration(ctx, writeFS, objPath, decl); err != nil {
			return err
		}
	}
	if err := WriteInventory(ctx, writeFS, objPath, inv); err != nil {
		return err
	}
	if err := WriteInventory(ctx, writeFS, path.Join(objPath, stage.vnum.String()), inv); err != nil {
		return err
	}
	return nil
}

// readExtensionConfig resolves the named extension and loads the extensions'
// configuration (if present) from the storage root extensions directory. If the
// extension's config file does not exist, no error is returned.
func (s *Store) readExtensionConfig(ctx context.Context, name string) (extensions.Extension, error) {
	ext, err := extensions.Get(name)
	if err != nil {
		return nil, err
	}
	err = readExtensionConfig(ctx, s.fsys, s.rootDir, ext)
	if err != nil {
		return nil, err
	}
	return ext, nil
}

// readExtensionConfig reads the extension config file for ext in the storage root's
// extensions directory. The value is unmarshalled into the value pointed to by
// ext. If the extension config does not exist, nil is returned.
func readExtensionConfig(ctx context.Context, fsys ocfl.FS, root string, ext extensions.Extension) error {
	confPath := path.Join(root, extensionsDir, ext.Name(), extensionConfigFile)
	f, err := fsys.OpenFile(ctx, confPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("reading config for extension %s: %w", ext.Name(), err)
		}
		return nil
	}
	defer f.Close()
	err = json.NewDecoder(f).Decode(ext)
	if err != nil {
		return fmt.Errorf("decoding config for extension %s: %w", ext.Name(), err)
	}
	return nil
}

// writeExtensionConfig writes the configuration files for the ext to the
// extensions directory in the storage root with at root.
func writeExtensionConfig(ctx context.Context, fsys ocfl.WriteFS, root string, ext extensions.Extension) error {
	confPath := path.Join(root, extensionsDir, ext.Name(), extensionConfigFile)
	b, err := json.Marshal(ext)
	if err != nil {
		return fmt.Errorf("encoding config for extension %s: %w", ext.Name(), err)
	}
	_, err = fsys.Write(ctx, confPath, bytes.NewBuffer(b))
	if err != nil {
		return fmt.Errorf("writing config for extension %s: %w", ext.Name(), err)
	}
	return nil
}

func newLayout(description string, layout extensions.Layout) storeLayout {
	return map[string]string{
		descriptionKey: description,
		extensionKey:   layout.Name(),
	}
}

// readLayout reads the `ocfl_layout.json` files in the storage root
// and unmarshals into the value pointed to by layout
func readLayout(ctx context.Context, fsys ocfl.FS, root string, layout *storeLayout) error {
	f, err := fsys.OpenFile(ctx, path.Join(root, layoutName))
	if err != nil {
		return fmt.Errorf("reading %s: %w", layoutName, err)
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(layout); err != nil {
		return fmt.Errorf("decoding %s: %w", layoutName, err)
	}
	return nil
}

// writeLayout marshals the value pointe to by layout and writes the result to
// the `ocfl_layout.json` files in the storage root.
func writeLayout(ctx context.Context, fsys ocfl.WriteFS, root string, layout storeLayout) error {
	b, err := json.Marshal(layout)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", layoutName, err)
	}
	_, err = fsys.Write(ctx, path.Join(root, layoutName), bytes.NewBuffer(b))
	if err != nil {
		return fmt.Errorf("writing %s: %w", layoutName, err)
	}
	return nil
}
