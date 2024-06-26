package ocflv1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/extension"
	"github.com/srerickson/ocfl-go/validation"
)

var (
	ErrLayoutUndefined = errors.New("storage root layout is undefined")
	ErrEmptyDirs       = errors.New("storage root includes empty directories")
	ErrNonObject       = errors.New("storage root includes files that aren't part of an object")
	ErrObjectVersion   = errors.New("storage root includes objects with higher OCFL version than the storage root")
)

// Store represents an existing OCFL v1.x Storage Root.
type Store struct {
	Layout extension.Layout

	fsys      ocfl.FS
	rootDir   string // storage root directory
	config    storeConfig
	spec      ocfl.Spec
	layoutErr error // error from ReadLayout()
}

// store layout represent ocfl_layout.json file
type storeConfig map[string]string

type InitStoreConf struct {
	Spec        ocfl.Spec
	Description string
	Layout      extension.Layout
	Extensions  []extension.Extension
}

// InitStore initializes a new OCFL v1.x storage root on fsys at root.
func InitStore(ctx context.Context, fsys ocfl.WriteFS, root string, conf *InitStoreConf) error {
	if conf == nil {
		conf = &InitStoreConf{}
	}
	if conf.Spec.Empty() {
		conf.Spec = defaultSpec
	}
	if !ocflVerSupported[conf.Spec] {
		return fmt.Errorf("%s: %w", conf.Spec, ErrOCFLVersion)
	}
	// default to 0002-flat-direct layout
	if conf.Layout == nil {
		conf.Layout = &extension.LayoutFlatDirect{}
	}
	decl := ocfl.Namaste{
		Type:    ocfl.NamasteTypeStore,
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
	if _, err := ocfl.WriteSpecFile(ctx, fsys, root, conf.Spec); err != nil {
		return err
	}
	layt := newStoreConfig(conf.Description, conf.Layout)
	if err := writeStoreConfig(ctx, fsys, root, layt); err != nil {
		return err
	}
	for _, e := range append(conf.Extensions, conf.Layout) {
		if err := writeExtensionConfig(ctx, fsys, root, e); err != nil {
			return err
		}
	}
	return nil
}

// Commit creates or updates the object with the given id using the contents of stage. The returned
// error is always a CommitError
func (s Store) Commit(ctx context.Context, id string, stage *ocfl.Stage, opts ...CommitOption) error {
	writeFS, ok := s.fsys.(ocfl.WriteFS)
	if !ok {
		return &CommitError{Err: fmt.Errorf("storage root backend is read-only")}
	}
	if s.Layout == nil {
		err := fmt.Errorf("commit requires a storage root layout: %w", ErrLayoutUndefined)
		return &CommitError{Err: err}
	}
	objPath, err := s.Layout.Resolve(id)
	if err != nil {
		return &CommitError{Err: fmt.Errorf("cannot commit id '%s': %w", id, err)}
	}
	// include the storage root spec so it can be checked during commit.
	opts = append(opts, withStoreSpec(s.spec))
	return Commit(ctx, writeFS, path.Join(s.rootDir, objPath), id, stage, opts...)
}

// GetStore returns a *Store for the OCFL Storage Root at root in fsys. The path
// root must be a directory/prefix with storage root declaration file.
func GetStore(ctx context.Context, fsys ocfl.FS, root string) (*Store, error) {
	// Don't use fs.ReadDir here as we would with GetObject because storage
	// roots might have huge numbers of directory entries. Instead, open storage
	// root declarations until we find one (or return error)
	var ocflVer ocfl.Spec
	for _, s := range []ocfl.Spec{ocflv1_1, ocflv1_0} {
		decl := ocfl.Namaste{Type: ocfl.NamasteTypeStore, Version: s}.Name()
		if err := ocfl.ValidateNamaste(ctx, fsys, path.Join(root, decl)); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("reading storage root delaration: %w", err)
		}
		ocflVer = s
		break
	}
	if ocflVer.Empty() {
		return nil, fmt.Errorf("missing storage root declaration: %w", ocfl.ErrNamasteNotExist)
	}
	str := &Store{
		fsys:    fsys,
		rootDir: root,
		spec:    ocflVer,
	}
	cfg := storeConfig{}
	err := readStoreConfig(ctx, fsys, root, &cfg)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		str.config = cfg
		// if ReadLayout fails, we don't return the error here. The store can
		// still be used, however, the error should be returned by ResolveID()
		// or other methods requiring the layout from the configuration.
		str.ReadLayout(ctx)
	}
	return str, nil
}

// Root returns the Store's ocfl.FS and root directory.
func (s *Store) Root() (ocfl.FS, string) {
	return s.fsys, s.rootDir
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

// Object returns a function iterator that yield objects in the storage root
func (s Store) Objects(ctx context.Context) ObjectSeq {
	return Objects(ctx, s.fsys, s.rootDir)
}

// Validate performs complete validation on the store
func (s *Store) Validate(ctx context.Context, opts ...ValidationOption) *validation.Result {
	return ValidateStore(ctx, s.fsys, s.rootDir, opts...)
}

// ResolveID resolves the storage path for the given id relative using the
// storage root's layout extension
func (s *Store) ResolveID(id string) (string, error) {
	if s.layoutErr != nil {
		return "", s.layoutErr
	}
	if s.Layout == nil {
		return "", ErrLayoutUndefined
	}
	return s.Layout.Resolve(id)
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

// ObjectExists returns true if an object with the given ID exists in the store
func (s *Store) ObjectExists(ctx context.Context, id string) (bool, error) {
	_, err := s.GetObject(ctx, id)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ReadLayout resolves the layout extension defined in ocfl_layout.json and
// loads its configuration file (if present) from the store's extensions
// directory. The store's active layout is set to the new layout. If no error is
// returned, subsequent calls to ResolveID() will resolve object ids using the
// new layout.
func (s *Store) ReadLayout(ctx context.Context) error {
	s.layoutErr = nil
	name := s.LayoutName()
	if name == "" {
		s.layoutErr = ErrLayoutUndefined
		return s.layoutErr
	}
	layout, err := readLayout(ctx, s.fsys, s.rootDir, name)
	if err != nil {
		s.layoutErr = err
		return s.layoutErr
	}
	s.Layout = layout
	return nil
}

func readLayout(ctx context.Context, fsys ocfl.FS, root string, name string) (extension.Layout, error) {
	ext, err := readExtensionConfig(ctx, fsys, root, name)
	if err != nil {
		return nil, err
	}
	layout, ok := ext.(extension.Layout)
	if !ok {
		return nil, extension.ErrNotLayout
	}
	return layout, nil
}

// readExtensionConfig reads the extension config file for ext in the storage root's
// extensions directory. The value is unmarshalled into the value pointed to by
// ext. If the extension config does not exist, nil is returned.
func readExtensionConfig(ctx context.Context, fsys ocfl.FS, root string, name string) (extension.Extension, error) {
	confPath := path.Join(root, extensionsDir, name, extensionConfigFile)
	f, err := fsys.OpenFile(ctx, confPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("openning config for extension %s: %w", name, err)
		}
		return nil, nil
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading config for extension %s: %w", name, err)
	}
	return extension.Unmarshal(b)
}

// writeExtensionConfig writes the configuration files for the ext to the
// extensions directory in the storage root with at root.
func writeExtensionConfig(ctx context.Context, fsys ocfl.WriteFS, root string, config extension.Extension) error {
	confPath := path.Join(root, extensionsDir, config.Name(), extensionConfigFile)
	b, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encoding config for extension %s: %w", config.Name(), err)
	}
	_, err = fsys.Write(ctx, confPath, bytes.NewBuffer(b))
	if err != nil {
		return fmt.Errorf("writing config for extension %s: %w", config.Name(), err)
	}
	return nil
}

func newStoreConfig(description string, layout extension.Layout) storeConfig {
	return map[string]string{
		descriptionKey: description,
		extensionKey:   layout.Name(),
	}
}

// readStoreConfig reads the `ocfl_layout.json` files in the storage root
// and unmarshals into the value pointed to by layout
func readStoreConfig(ctx context.Context, fsys ocfl.FS, root string, layout *storeConfig) error {
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

// writeStoreConfig marshals the value pointe to by layout and writes the result to
// the `ocfl_layout.json` files in the storage root.
func writeStoreConfig(ctx context.Context, fsys ocfl.WriteFS, root string, layout storeConfig) error {
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
