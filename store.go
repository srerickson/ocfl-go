package ocfl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/srerickson/ocfl-go/extension"
)

const (
	layoutName          = "ocfl_layout.json"
	descriptionKey      = `description`
	extensionKey        = `extension`
	extensionConfigFile = "config.json"
)

var (
	ErrLayoutUndefined = errors.New("storage root's layout is undefined")
)

// Store represents an OCFL Storage Root.
type Store struct {
	fs         FS                // root's fs
	dir        string            // root's director relative to FS
	exists     bool              // storage root exists or not
	spec       Spec              // OCFL spec version in storage root declaration
	layout     extension.Layout  // layout used to resolve object ids
	layoutConf map[string]string // contents of `ocfl_layout.json`
}

type StoreOption func(*Store)

func NewStore(ctx context.Context, fsys FS, dir string, opts ...StoreOption) (*Store, error) {
	entries, err := fsys.ReadDir(ctx, dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if len(entries) < 1 {
		// return uninitialized store
		return &Store{fs: fsys, dir: dir}, nil
	}
	decl, err := FindNamaste(entries)
	if err == nil && decl.Type != NamasteTypeStore {
		err = fmt.Errorf("NAMASTE declaration has wrong type: %q", decl.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("not an OCFL storage root: %w", err)
	}
	// initialize a new existing Store
	s := &Store{fs: fsys, dir: dir, spec: decl.Version, exists: true}
	// set storage root's layout if possible
	if err := s.getLayout(ctx); err != nil {
		if !errors.Is(err, ErrLayoutUndefined) {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) Init(ctx context.Context, spec Spec, layoutDesc string, extensions ...extension.Extension) error {
	if s.exists {
		return errors.New("can't initialize already existing root")
	}
	writeFS, isWriteFS := s.fs.(WriteFS)
	if !isWriteFS {
		return fmt.Errorf("storage root backend is not writable")
	}
	decl := Namaste{Version: spec, Type: NamasteTypeStore}
	if err := WriteDeclaration(ctx, writeFS, s.dir, decl); err != nil {
		return err
	}
	var haveLayout bool
	for _, e := range extensions {
		layout, isLayout := e.(extension.Layout)
		if isLayout && !haveLayout {
			if err := s.setLayout(ctx, layout, layoutDesc); err != nil {
				return err
			}
			haveLayout = true
			continue
		}
		if err := writeExtensionConfig(ctx, writeFS, s.dir, e); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Description() string {
	return s.layoutConf[descriptionKey]
}

func (s *Store) FS() FS {
	return s.fs
}

func (s *Store) Exists() bool {
	return s.exists
}

func (s *Store) NewObject(ctx context.Context, id string, opts ...ObjectOption) (*Object, error) {
	if s.layout == nil {
		return nil, ErrLayoutUndefined
	}
	objPath, err := s.layout.Resolve(id)
	if err != nil {
		return nil, fmt.Errorf("object id: %q: %w", id, err)
	}
	if !fs.ValidPath(objPath) {
		return nil, fmt.Errorf("layout resolved id to an invalid path: %s", objPath)
	}
	return NewObject(ctx, s.fs, path.Join(s.dir, objPath), opts...)
}

func (s *Store) Path() string {
	return s.dir
}

func (s *Store) Spec() Spec {
	return s.spec
}

func (s *Store) getLayout(ctx context.Context) error {
	s.layoutConf = nil
	s.layout = nil
	if err := s.readLayoutConfig(ctx); err != nil {
		return err
	}
	if s.layoutConf == nil || s.layoutConf[extensionKey] == "" {
		return ErrLayoutUndefined
	}
	name := s.layoutConf[extensionKey]
	ext, err := readExtensionConfig(ctx, s.fs, s.dir, name)
	if err != nil {
		return err
	}
	layout, ok := ext.(extension.Layout)
	if !ok {
		return fmt.Errorf("extension: %q: %w", name, extension.ErrNotLayout)
	}
	s.layout = layout
	return nil
}

// readLayoutConfig reads the `ocfl_layout.json` files in the storage root
// and unmarshals into the value pointed to by layout
func (s *Store) readLayoutConfig(ctx context.Context) error {
	f, err := s.fs.OpenFile(ctx, path.Join(s.dir, layoutName))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			s.layoutConf = nil
			return nil
		}
		return err
	}
	defer f.Close()
	layout := map[string]string{}
	if err := json.NewDecoder(f).Decode(&layout); err != nil {
		return fmt.Errorf("decoding %s: %w", layoutName, err)
	}
	s.layoutConf = layout
	return nil
}

// setLayout marshals the value pointe to by layout and writes the result to
// the `ocfl_layout.json` files in the storage root.
func (s *Store) setLayout(ctx context.Context, layout extension.Layout, desc string) error {
	writeFS, isWriteFS := s.fs.(WriteFS)
	if !isWriteFS {
		return fmt.Errorf("storage root backend is not writable")
	}
	layoutPath := path.Join(s.dir, layoutName)
	if layout == nil {
		s.layout = nil
		s.layoutConf = nil
		if s.exists {
			// remove existing file
			if err := writeFS.Remove(ctx, layoutPath); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
		}
		return nil
	}
	newConfig := map[string]string{
		extensionKey:   layout.Name(),
		descriptionKey: desc,
	}
	b, err := json.Marshal(newConfig)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", layoutName, err)
	}
	_, err = writeFS.Write(ctx, layoutPath, bytes.NewBuffer(b))
	if err != nil {
		return fmt.Errorf("writing %s: %w", layoutName, err)
	}
	if err := writeExtensionConfig(ctx, writeFS, s.dir, layout); err != nil {
		return fmt.Errorf("setting root layout extension: %w", err)
	}
	s.layoutConf = newConfig
	s.layout = layout
	return nil
}

// readExtensionConfig reads the extension config file for ext in the storage root's
// extensions directory. The value is unmarshalled into the value pointed to by
// ext. If the extension config does not exist, nil is returned.
func readExtensionConfig(ctx context.Context, fsys FS, root string, name string) (extension.Extension, error) {
	confPath := path.Join(root, ExtensionsDir, name, extensionConfigFile)
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
func writeExtensionConfig(ctx context.Context, fsys WriteFS, root string, config extension.Extension) error {
	confPath := path.Join(root, ExtensionsDir, config.Name(), extensionConfigFile)
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
