package ocfl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"path"

	"github.com/srerickson/ocfl-go/extension"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	"github.com/srerickson/ocfl-go/internal/pipeline"
)

const (
	layoutConfigFile    = "ocfl_layout.json"
	descriptionKey      = `description`
	extensionKey        = `extension`
	extensionConfigFile = "config.json"
)

var ErrLayoutUndefined = errors.New("storage root's layout is undefined")

// Root represents an OCFL Storage Root.
type Root struct {
	fs           ocflfs.FS         // root's fs
	dir          string            // root's director relative to FS
	spec         Spec              // OCFL spec version in storage root declaration
	layout       extension.Layout  // layout used to resolve object ids
	layoutConfig map[string]string // contents of `ocfl_layout.json`

	// initArgs is used to initialize new root. Values
	// are set by InitRoot option.
	initArgs *initRootArgs
}

// NewRoot returns a new *[Root] for working with the OCFL storage root at
// directory dir in fsys. It can be used to initialize new storage roots if the
// [InitRoot] option is used, fsys is an ocfl.WriteFS, and dir is a non-existing
// or empty directory.
func NewRoot(ctx context.Context, fsys ocflfs.FS, dir string, opts ...RootOption) (*Root, error) {
	r := &Root{fs: fsys, dir: dir}
	for _, opt := range opts {
		opt(r)
	}
	entries, err := ocflfs.ReadDir(ctx, fsys, dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	// try to inititializing a new storage root
	if len(entries) < 1 && r.initArgs != nil {
		if err := r.init(ctx); err != nil {
			return nil, fmt.Errorf("initializing new storage root: %w", err)
		}
		return r, nil
	}
	// find storage root declaration
	decl, err := FindNamaste(entries)
	if err == nil && decl.Type != NamasteTypeRoot {
		err = fmt.Errorf("NAMASTE declaration has wrong type: %q", decl.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("not an OCFL storage root: %w", err)
	}
	if _, err := getOCFL(decl.Version); err != nil {
		return nil, fmt.Errorf(" OCFL v%s: %w", decl.Version, err)
	}
	// initialize existing Root
	r.spec = decl.Version
	if err := r.getLayout(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

// Description returns the description string from the storage roots
// `ocfl_layout.json` file, which may be empty.
func (r *Root) Description() string {
	return r.layoutConfig[descriptionKey]
}

// FS returns the Root's FS
func (r *Root) FS() ocflfs.FS {
	return r.fs
}

// Layout returns the storage root's layout, which may be nil.
func (r *Root) Layout() extension.Layout {
	return r.layout
}

// LayoutName returns the name of the root's layout extension or an empty string
// if the root has no layout.
func (r *Root) LayoutName() string {
	if r.layout == nil {
		return ""
	}
	return r.layout.Name()
}

// NewObject returns an *Object for managing the OCFL object with the given ID
// in the root. If the object does not exist, the returned *Object can be used
// to create it. If the Root has not storage layout for resovling object IDs,
// the returned error is ErrLayoutUndefined.
func (r *Root) NewObject(ctx context.Context, id string, opts ...ObjectOption) (*Object, error) {
	objPath, err := r.ResolveID(id)
	if err != nil {
		return nil, err
	}
	opts = append(opts, ObjectWithID(id))
	return r.NewObjectDir(ctx, objPath, opts...)
}

// NewObjectDir returns an *Object for managing the OCFL object at path dir in
// root. If the object does not exist, the returned *Object can be used to
// create it.
func (r *Root) NewObjectDir(ctx context.Context, dir string, opts ...ObjectOption) (*Object, error) {
	opts = append(opts, objectWithRoot(r))
	objPath := path.Join(r.dir, dir)
	return NewObject(ctx, r.fs, objPath, opts...)
}

// ResolveID resolves the object id to a path relative to the root. If
// the root has no layout, the returned error is ErrLayoutUndefined.
func (r *Root) ResolveID(id string) (string, error) {
	if r.layout == nil {
		return "", ErrLayoutUndefined
	}
	objPath, err := r.layout.Resolve(id)
	if err != nil {
		return "", fmt.Errorf("object id: %q: %w", id, err)
	}
	if !fs.ValidPath(objPath) {
		return "", fmt.Errorf("layout resolved id to an invalid path: %s", objPath)
	}
	return objPath, nil
}

// ObjectDeclarations returns an iterator that yields all OCFL object
// declaration files in r. If an error occurs during iteration, it is returned
// by the error function.
func (r *Root) ObjectDeclarations(ctx context.Context) iter.Seq2[*ocflfs.FileRef, error] {
	return func(yield func(*ocflfs.FileRef, error) bool) {
		for f, err := range ocflfs.WalkFiles(ctx, r.fs, r.dir) {
			if err != nil {
				yield(nil, err)
				return
			}
			decl, err := ParseNamaste(path.Base(f.Path))
			if err == nil && decl.IsObject() {
				if !yield(f, nil) {
					return
				}
			}
		}
	}
}

// Objects returns an iterator that yields objects or an error for every object
// declaration file in the root. Objects are yielded in arbitrary order.
func (r *Root) Objects(ctx context.Context, opts ...ObjectOption) iter.Seq2[*Object, error] {
	return r.ObjectsBatch(ctx, 0, opts...)
}

// ObjectsBatch returns an iterator that yields objects or an error for every object
// declaration file in the root. Objects are read in numgos go routines and are
// yielded in arbitrary order.
func (r *Root) ObjectsBatch(ctx context.Context, numgos int, opts ...ObjectOption) iter.Seq2[*Object, error] {
	return func(yield func(*Object, error) bool) {
		opts = append(opts, ObjectMustExist(), objectWithRoot(r))
		openObj := func(ref *ocflfs.FileRef) (*Object, error) {
			return NewObject(ctx, ref.FS, ref.FullPathDir(), opts...)
		}
		declFiles, errFn := ocflfs.UntilErr(r.ObjectDeclarations(ctx))
		for result := range pipeline.Results(declFiles, openObj, numgos) {
			if !yield(result.Out, result.Err) {
				return
			}
		}
		if err := errFn(); err != nil {
			yield(nil, err)
		}
	}
}

// Path returns the root's dir relative to its FS
func (r *Root) Path() string {
	return r.dir
}

// Spec returns the root's OCFL specification number
func (r *Root) Spec() Spec {
	return r.spec
}

// ValidateObject validates the object with the given id. If the id cannot be
// resolved, the error is reported as a fatal error in the returned
// *ObjectValidation.
func (r *Root) ValidateObject(ctx context.Context, id string, opts ...ObjectValidationOption) *ObjectValidation {
	objPath, err := r.ResolveID(id)
	if err != nil {
		v := newObjectValidation(r.fs, path.Join(r.dir, objPath), opts...)
		v.AddFatal(err)
		return v
	}
	return r.ValidateObjectDir(ctx, objPath, opts...)
}

// ValidateObjectDir validates the object at a path relative to the root.
func (r *Root) ValidateObjectDir(ctx context.Context, dir string, opts ...ObjectValidationOption) *ObjectValidation {
	return ValidateObject(ctx, r.fs, path.Join(r.dir, dir), opts...)
}

func (r *Root) init(ctx context.Context) error {
	if r.initArgs == nil {
		return nil
	}
	if r.initArgs.spec.Empty() {
		return errors.New("can't initialize storage root: missing OCFL spec version")
	}
	if _, err := getOCFL(r.initArgs.spec); err != nil {
		return fmt.Errorf(" OCFL v%s: %w", r.initArgs.spec, err)
	}
	writeFS, isWriteFS := r.fs.(ocflfs.WriteFS)
	if !isWriteFS {
		return fmt.Errorf("storage root backend is not writable")
	}
	decl := Namaste{Version: r.initArgs.spec, Type: NamasteTypeRoot}
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

func (r *Root) getLayout(ctx context.Context) error {
	r.layoutConfig = nil
	r.layout = nil
	if err := r.readLayoutConfig(ctx); err != nil {
		return err
	}
	if r.layoutConfig == nil || r.layoutConfig[extensionKey] == "" {
		return nil
	}
	name := r.layoutConfig[extensionKey]
	ext, err := readExtensionConfig(ctx, r.fs, r.dir, name)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		// Allow for missing extension config: If the config doesn't exist, use
		// the extensions default values.
		ext, err = extension.Get(name)
		if err != nil {
			return err
		}
	}
	layout, ok := ext.(extension.Layout)
	if !ok {
		return fmt.Errorf("extension: %q: %w", name, extension.ErrNotLayout)
	}
	r.layout = layout
	return nil
}

// readLayoutConfig reads the `ocfl_layout.json` files in the storage root
// and unmarshals into the value pointed to by layout
func (r *Root) readLayoutConfig(ctx context.Context) error {
	f, err := r.fs.OpenFile(ctx, path.Join(r.dir, layoutConfigFile))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			r.layoutConfig = nil
			return nil
		}
		return err
	}
	defer f.Close()
	layout := map[string]string{}
	if err := json.NewDecoder(f).Decode(&layout); err != nil {
		return fmt.Errorf("decoding %s: %w", layoutConfigFile, err)
	}
	r.layoutConfig = layout
	return nil
}

// setLayout marshals the value pointed to by layout and writes the result to
// the `ocfl_layout.json` files in the storage root.
func (r *Root) setLayout(ctx context.Context, layout extension.Layout, desc string) error {
	writeFS, isWriteFS := r.fs.(ocflfs.WriteFS)
	if !isWriteFS {
		return fmt.Errorf("storage root backend is not writable")
	}
	layoutPath := path.Join(r.dir, layoutConfigFile)
	if layout == nil && r.layout != nil {
		r.layout = nil
		r.layoutConfig = nil
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
		return fmt.Errorf("encoding %s: %w", layoutConfigFile, err)
	}
	_, err = writeFS.Write(ctx, layoutPath, bytes.NewBuffer(b))
	if err != nil {
		return fmt.Errorf("writing %s: %w", layoutConfigFile, err)
	}
	if err := writeExtensionConfig(ctx, writeFS, r.dir, layout); err != nil {
		return fmt.Errorf("setting root layout extension: %w", err)
	}
	r.layoutConfig = newConfig
	r.layout = layout
	return nil
}

// readExtensionConfig reads the extension config file for ext in the storage root's
// extensions directory. The value is unmarshaled into the value pointed to by
// ext. If the extension config does not exist, nil is returned.
func readExtensionConfig(ctx context.Context, fsys ocflfs.FS, root string, name string) (extension.Extension, error) {
	confPath := path.Join(root, extensionsDir, name, extensionConfigFile)
	f, err := fsys.OpenFile(ctx, confPath)
	if err != nil {
		return nil, fmt.Errorf("can't open config for extension %s: %w", name, err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading config for extension %s: %w", name, err)
	}
	return extension.DefaultRegistry().Unmarshal(b)
}

// writeExtensionConfig writes the configuration files for the ext to the
// extensions directory in the storage root with at root.
func writeExtensionConfig(ctx context.Context, fsys ocflfs.WriteFS, root string, config extension.Extension) error {
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

// RootOption is used to configure the behavior of [NewRoot]()
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
