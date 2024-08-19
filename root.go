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

// Root represents an OCFL Storage Root.
type Root struct {
	fs         FS                // root's fs
	dir        string            // root's director relative to FS
	global     Config            // shared OCFL settings
	spec       Spec              // OCFL spec version in storage root declaration
	layout     extension.Layout  // layout used to resolve object ids
	layoutConf map[string]string // contents of `ocfl_layout.json`

	// initArgs is used to initialize new root. Values
	// are set by InitRoot option.
	initArgs *initRootArgs
}

func NewRoot(ctx context.Context, fsys FS, dir string, opts ...RootOption) (*Root, error) {
	r := &Root{fs: fsys, dir: dir}
	for _, opt := range opts {
		opt(r)
	}

	entries, err := fsys.ReadDir(ctx, dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	// try to inititializing a new storage root
	if len(entries) < 1 && r.initArgs != nil {
		if err := r.init(ctx); err != nil {
			return nil, fmt.Errorf("while creating a new storage root: %w", err)
		}
		return r, nil
	}
	// find storage root declaration
	decl, err := FindNamaste(entries)
	if err == nil && decl.Type != NamasteTypeStore {
		err = fmt.Errorf("NAMASTE declaration has wrong type: %q", decl.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("not an OCFL storage root: %w", err)
	}
	if _, err := r.global.GetSpec(decl.Version); err != nil {
		return nil, fmt.Errorf(" OCFL v%s: %w", decl.Version, err)
	}
	// initialize existing Root
	r.spec = decl.Version
	if err := r.getLayout(ctx); err != nil {
		if !errors.Is(err, ErrLayoutUndefined) {
			return nil, err
		}
	}
	return r, nil
}

func (s *Root) Description() string {
	return s.layoutConf[descriptionKey]
}

func (s *Root) FS() FS {
	return s.fs
}

func (r *Root) Layout() extension.Layout {
	return r.layout
}

// NewObject returns a new
func (s *Root) NewObject(ctx context.Context, id string, opts ...ObjectOption) (*Object, error) {
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
	opts = append(opts, objectExpectedID(id))
	return NewObject(ctx, s.fs, path.Join(s.dir, objPath), opts...)
}

// ObjectPaths returns an iterator of object paths for the objects in the
// storage root.
func (r *Root) ObjectPaths(ctx context.Context) func(func(string, error) bool) {
	return ObjectPaths(ctx, r.fs, r.dir)
}

// Objects returns an iterator of objects in the storage root.
func (r *Root) Objects(ctx context.Context) func(func(*Object, error) bool) {
	return func(yield func(*Object, error) bool) {
		for dir, err := range r.ObjectPaths(ctx) {
			if err != nil {
				yield(nil, err)
				return
			}
			obj, err := NewObject(ctx, r.fs, dir)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(obj, nil) {
				return
			}
		}
	}
}

func (s *Root) Path() string {
	return s.dir
}

func (s *Root) Spec() Spec {
	return s.spec
}

func (r *Root) init(ctx context.Context) error {
	if r.initArgs == nil {
		return nil
	}
	if r.initArgs.spec.Empty() {
		return errors.New("can't initialize storage root: missing OCFL spec version")
	}
	if _, err := r.global.GetSpec(r.initArgs.spec); err != nil {
		return fmt.Errorf(" OCFL v%s: %w", r.initArgs.spec, err)
	}
	writeFS, isWriteFS := r.fs.(WriteFS)
	if !isWriteFS {
		return fmt.Errorf("storage root backend is not writable")
	}
	decl := Namaste{Version: r.initArgs.spec, Type: NamasteTypeStore}
	if err := WriteDeclaration(ctx, writeFS, r.dir, decl); err != nil {
		return err
	}
	r.spec = r.initArgs.spec
	var haveLayout bool
	for _, e := range r.initArgs.extensions {
		layout, isLayout := e.(extension.Layout)
		if isLayout && !haveLayout {
			if err := r.setLayout(ctx, layout, r.initArgs.layoutDesc); err != nil {
				return err
			}
			haveLayout = true
			continue
		}
		if err := writeExtensionConfig(ctx, writeFS, r.dir, e); err != nil {
			return err
		}
	}
	return nil
}

func (s *Root) getLayout(ctx context.Context) error {
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
func (s *Root) readLayoutConfig(ctx context.Context) error {
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
func (r *Root) setLayout(ctx context.Context, layout extension.Layout, desc string) error {
	writeFS, isWriteFS := r.fs.(WriteFS)
	if !isWriteFS {
		return fmt.Errorf("storage root backend is not writable")
	}
	layoutPath := path.Join(r.dir, layoutName)
	if layout == nil && r.layout != nil {
		r.layout = nil
		r.layoutConf = nil
		// remove existing file
		if err := writeFS.Remove(ctx, layoutPath); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
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
	if err := writeExtensionConfig(ctx, writeFS, r.dir, layout); err != nil {
		return fmt.Errorf("setting root layout extension: %w", err)
	}
	r.layoutConf = newConfig
	r.layout = layout
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

type RootOption func(*Root)

type initRootArgs struct {
	spec       Spec
	layoutDesc string
	extensions []extension.Extension
}

// InitRoot returns a RootOption for initializing a new storage root as part of
// the call to NewRoot().
func InitRoot(spec Spec, layoutDesc string, extensions ...extension.Extension) RootOption {
	return func(root *Root) {
		root.initArgs = &initRootArgs{
			spec:       spec,
			layoutDesc: layoutDesc,
			extensions: extensions,
		}
	}
}
