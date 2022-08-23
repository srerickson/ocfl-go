package ocflv1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl"
	"github.com/srerickson/ocfl/backend"
	"github.com/srerickson/ocfl/extensions"
)

var ErrLayoutUndefined = errors.New("storage root layout is undefined")

// Store represents an existing OCFL v1.x Storage Root. It supports read-only
// access.
type Store struct {
	Config      *StoreLayout
	fsys        ocfl.FS
	rootDir     string // storage root
	ocflVersion ocfl.Spec
	getPath     extensions.LayoutFunc
}

// GetStore returns a *Store for the OCFL Storage Root at root in fsys. The path
// root must be a directory/prefix with storage root declaration file. The
// returned store's active layout is not set -- it should be set with SetLayout()
// or ReadLayout() before using GetID().
func GetStore(ctx context.Context, fsys ocfl.FS, root string) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dirList, err := fsys.ReadDir(ctx, root)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	str := &Store{
		fsys:        fsys,
		rootDir:     root,
		ocflVersion: ocflVer,
	}
	for _, inf := range dirList {
		if inf.Type().IsRegular() && inf.Name() == layoutName {
			str.Config = &StoreLayout{}
			err = ReadLayout(fsys, root, str.Config)
			if err != nil {
				return nil, err
			}
			break
		}
	}
	return str, nil
}

// Descriptions returns the description set in the storage root's
// ocfl_layout.json files, or an empty string if the description is undefined
func (s *Store) Description() string {
	if s.Config == nil {
		return ""
	}
	return s.Config.Description()
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

// GetPath returns an Object for the given path
func (s *Store) GetPath(ctx context.Context, p string) (*Object, error) {
	return GetObject(ctx, s.fsys, path.Join(s.rootDir, p))
}

// GetID resolves the path for the OCFL object id using the store's layout
// extension (if defined). The store layout is set during GetStore() if the
// storage root includes an ocfl_layout.json file. Otherwise, it can be set
// using, SetLayout().
func (s *Store) GetID(ctx context.Context, id string) (*Object, error) {
	if s.getPath == nil {
		return nil, ErrLayoutUndefined
	}
	pth, err := s.getPath(id)
	if err != nil {
		return nil, err
	}
	return GetObject(ctx, s.fsys, path.Join(s.rootDir, pth))
}

// SetLayout sets the store's active layout. If no error is returned, subsequent
// calls to GetID() will resolve object ids using the new layout.
func (s *Store) SetLayout(layout extensions.Layout) error {
	getPath, err := layout.NewFunc()
	if err != nil {
		return err
	}
	s.getPath = getPath
	return nil
}

// ReadLayout resolves the named layout extension and loads its configuration
// file (if present) from the store's extensions directory. The store's active
// layout is set to the new layout. If no error is returned, subsequent calls to
// GetID() will resolve object ids using the new layout.
func (s *Store) ReadLayout(ctx context.Context, name string) error {
	ext, err := s.readExtension(ctx, name)
	if err != nil {
		return err
	}
	layoutExt, ok := ext.(extensions.Layout)
	if !ok {
		return fmt.Errorf("%s: %w", name, extensions.ErrNotLayout)
	}
	return s.SetLayout(layoutExt)
}

// readExtension resolves the named extension and loads the extensions'
// configuration (if present) from the storage root extensions directory. If the
// extension's config file does not exist, no error is returned.
func (s *Store) readExtension(ctx context.Context, name string) (extensions.Extension, error) {
	ext, err := extensions.Get(name)
	if err != nil {
		return nil, err
	}
	err = ReadExtensionConfig(ctx, s.fsys, s.rootDir, ext)
	if err != nil {
		return nil, err
	}
	return ext, nil
}

// StoreLayout represents an OCFL v1.X storage root ocfl_layout.json file
type StoreLayout struct {
	values map[string]string
}

// NewStoreLayout returns a new *StoreLayout with the specified description and
// layout extension
func NewStoreLayout(description, extension string) *StoreLayout {
	return &StoreLayout{
		values: map[string]string{
			descriptionKey: description,
			extensionKey:   extension,
		},
	}
}

func (l *StoreLayout) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &l.values)
}

func (l StoreLayout) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.values)
}

// Description returns the extension name and a boolean indicating if the value is
// defined in the ocfl_layout.json file
func (l StoreLayout) Description() string {
	if l.values == nil {
		return ""
	}
	return l.values[descriptionKey]
}

// Extension returns the extension name and a boolean indicating if the value is
// defined in the ocfl_layout.json file.
func (l StoreLayout) Extension() string {
	if l.values == nil {
		return ""
	}
	return l.values[extensionKey]
}

// ReadExtensionConfig reads the extension config file for ext in the storage root's
// extensions directory. The value is unmarshalled into the value pointed to by
// ext. If the extension config does not exist, nil is returned.
func ReadExtensionConfig(ctx context.Context, fsys ocfl.FS, root string, ext extensions.Extension) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	confPath := path.Join(root, extensionsDir, ext.Name(), extensionConfigFile)
	f, err := fsys.OpenFile(ctx, confPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: %w", ext.Name(), err)
		}
		return nil
	}
	err = json.NewDecoder(f).Decode(&ext)
	if err != nil {
		return err
	}
	return nil
}

// WriteExtensionConfig writes the configuration files for the ext to the
// extensions directory in the storage root with at root.
func WriteExtensionConfig(fsys backend.Writer, root string, ext extensions.Extension) error {
	confPath := path.Join(root, extensionsDir, ext.Name(), extensionConfigFile)
	b, err := json.Marshal(ext)
	if err != nil {
		return err
	}
	_, err = fsys.Write(confPath, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	return nil
}

// ReadLayout reads the `ocfl_layout.json` files in the storage root
// and unmarshals into the value pointed to by layout
func ReadLayout(fsys ocfl.FS, root string, layout *StoreLayout) error {
	f, err := fsys.OpenFile(context.TODO(), path.Join(root, layoutName))
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(layout)
}

// WriteLayout marshals the value pointe to by layout and writes the result to
// the `ocfl_layout.json` files in the storage root.
func WriteLayout(fsys backend.Writer, root string, layout *StoreLayout) error {
	b, err := json.Marshal(layout)
	if err != nil {
		return err
	}
	_, err = fsys.Write(path.Join(root, layoutName), bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	return nil
}
