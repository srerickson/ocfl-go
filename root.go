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
	"strings"

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

// NewObjectUpdate returns a new draft *[ObjectUpdate] for the object with the
// given ID in the root and an *[Object] for the object, as with
// [NewObjectUpdate]. The root's OCFL spec is recorded in the update, so the
// update can't give the object a newer spec than the root's (E081). A new
// object uses the root's spec unless [FinalizeWithOCFLSpec] is used. The
// object's Root method returns r. If the Root has no storage layout for
// resolving object IDs, the returned error is ErrLayoutUndefined.
func (r *Root) NewObjectUpdate(ctx context.Context, id string, opts ...NewUpdateOption) (*ObjectUpdate, *Object, error) {
	objPath, err := r.ResolveID(id)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, newUpdateWithRootSpec(r.spec))
	u, obj, err := NewObjectUpdate(ctx, r.fs, path.Join(r.dir, objPath), id, opts...)
	if err != nil {
		return nil, nil, err
	}
	obj.root = r
	return u, obj, nil
}

// Apply applies the finalized update u to the object in the root with u's
// ID, using src for new content, and returns the updated object. See
// [ObjectUpdate.Apply]. Apply returns an error wrapping
// [ErrObjectSpecExceedsRoot], without writing anything, if u's new version
// has a newer OCFL spec than the root's (E081). This can happen if u wasn't
// created with [Root.NewObjectUpdate].
func (r *Root) Apply(ctx context.Context, u *ObjectUpdate, src ContentSource, opts ...ApplyUpdateOption) (*Object, error) {
	if info, ok := u.VersionInfo(); ok && !r.spec.Empty() && info.Spec.Cmp(r.spec) > 0 {
		return nil, fmt.Errorf("%w: object spec is OCFL v%s, storage root spec is OCFL v%s",
			ErrObjectSpecExceedsRoot, info.Spec, r.spec)
	}
	objPath, err := r.ResolveID(u.ID())
	if err != nil {
		return nil, err
	}
	obj, err := u.Apply(ctx, r.fs, path.Join(r.dir, objPath), src, opts...)
	if err != nil {
		return nil, err
	}
	obj.root = r
	return obj, nil
}

// Revert reverts the interrupted update u to the object in the root with u's
// ID. See [ObjectUpdate.Revert].
func (r *Root) Revert(ctx context.Context, u *ObjectUpdate, opts ...ApplyUpdateOption) error {
	objPath, err := r.ResolveID(u.ID())
	if err != nil {
		return err
	}
	return u.Revert(ctx, r.fs, path.Join(r.dir, objPath), opts...)
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
// declaration files in r. Storage hierarchies terminate at object roots, so
// declarations nested inside another object's directory (for example, an
// object stored as content of another object) are skipped. If an error occurs
// during iteration, it is yielded and iteration stops.
//
// If r's FS is an [ocflfs.FileWalker], declarations are found in its flat
// listing, which must be in one of the orders FileWalker documents. A listing
// found to be in neither order is reported as an error; declarations yielded
// before it may include nested ones.
func (r *Root) ObjectDeclarations(ctx context.Context) iter.Seq2[*ocflfs.FileRef, error] {
	if _, ok := r.fs.(ocflfs.FileWalker); ok {
		return r.listObjectDeclarations(ctx)
	}
	return func(yield func(*ocflfs.FileRef, error) bool) {
		r.walkObjectDeclarations(ctx, ".", yield)
	}
}

// walkObjectDeclarations yields the object declarations in dir and below,
// relative to r.dir. It reads each directory's entries before descending, so
// it doesn't descend into a directory with an object declaration, whatever
// order the entries are in. The root directory itself is always descended:
// it isn't an object root, even if it has a stray object declaration.
func (r *Root) walkObjectDeclarations(ctx context.Context, dir string, yield func(*ocflfs.FileRef, error) bool) bool {
	var subdirs []string
	var isObject bool
	for e, err := range ocflfs.DirEntries(ctx, r.fs, path.Join(r.dir, dir)) {
		if err != nil {
			yield(nil, err)
			return false
		}
		if e == nil {
			continue
		}
		if e.IsDir() {
			subdirs = append(subdirs, path.Join(dir, e.Name()))
			continue
		}
		decl, err := ParseNamaste(e.Name())
		if err != nil || !decl.IsObject() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			yield(nil, err)
			return false
		}
		isObject = true
		ref := &ocflfs.FileRef{
			FS:      r.fs,
			BaseDir: r.dir,
			Path:    path.Join(dir, e.Name()),
			Info:    info,
		}
		if !yield(ref, nil) {
			return false
		}
	}
	if isObject && dir != "." {
		return true
	}
	for _, sub := range subdirs {
		if !r.walkObjectDeclarations(ctx, sub, yield) {
			return false
		}
	}
	return true
}

// listObjectDeclarations yields the object declarations from a flat listing of
// every file under r.dir, as [ocflfs.FileWalker] provides (e.g., S3). It
// relies on the listing order that FileWalker documents: either full-path
// lexical order or depth-first order with sorted entry names. In both, the
// files under any directory form one contiguous run of the listing, which
// lets it track only the directories on the path to the current file. It
// yields an error if the listing turns out to be in neither order.
func (r *Root) listObjectDeclarations(ctx context.Context) iter.Seq2[*ocflfs.FileRef, error] {
	return func(yield func(*ocflfs.FileRef, error) bool) {
		l := declLister{yield: yield}
		var order listOrder
		for f, err := range ocflfs.WalkFiles(ctx, r.fs, r.dir) {
			if err != nil {
				yield(nil, err)
				return
			}
			if err := order.next(f.Path); err != nil {
				yield(nil, fmt.Errorf("listing object declarations in %q: %w", r.dir, err))
				return
			}
			if !l.next(f) {
				return
			}
		}
		l.finish()
	}
}

// declLister finds the object declarations in an ordered flat listing that
// aren't nested inside another declared object's directory.
//
// It keeps a frame for each directory on the path to the current file, from
// the shallowest (below the root) to the deepest. A directory's object
// declaration can come after files under it in the listing: an invalid object
// root can have entries that sort before "0=ocfl_object_". So a declaration
// with an ancestor that might still turn out to be declared is held, attached
// to the deepest such ancestor's frame. The invariants are:
//
//   - A frame is resolved once its directory's declaration has been seen
//     (declared) or the listing has moved past every position its declaration
//     could have (passed). Frames that are left are resolved as passed.
//   - A held declaration is attached to its deepest ancestor frame that is
//     unresolved, and has no declared ancestor.
//   - No frame below a declared frame holds anything.
//
// When a frame resolves as passed, its held declarations move to the next
// unresolved frame up, or are yielded if there is none. When a frame is
// declared, everything it holds is dropped. Memory is bounded by the depth of
// the tree plus the number of declarations currently held.
type declLister struct {
	yield  func(*ocflfs.FileRef, error) bool
	frames []declFrame
}

type declFrame struct {
	dir      string // directory path, relative to the walked dir
	declared bool   // dir has an object declaration
	passed   bool   // the listing is past where dir's declaration could be
	held     []*ocflfs.FileRef
}

// next processes the next file in the listing. It returns false if iteration
// should stop.
func (l *declLister) next(f *ocflfs.FileRef) bool {
	name := f.Path
	// leave frames for directories that don't contain name: since each
	// directory's files are contiguous in the listing, nothing more can come
	// under them.
	for len(l.frames) > 0 {
		top := l.frames[len(l.frames)-1]
		if isUnder(name, top.dir) {
			break
		}
		l.frames = l.frames[:len(l.frames)-1]
		if !top.declared && !l.release(len(l.frames), top.held) {
			return false
		}
	}
	// enter frames for the rest of name's directories.
	start := 0
	if len(l.frames) > 0 {
		start = len(l.frames[len(l.frames)-1].dir) + 1
	}
	for i := start; i < len(name); i++ {
		if name[i] == '/' {
			l.frames = append(l.frames, declFrame{dir: name[:i]})
		}
	}
	// resolve frames the listing has now passed, deepest first so held
	// declarations can move up through several frames at once.
	for i := len(l.frames) - 1; i >= 0; i-- {
		fr := &l.frames[i]
		if fr.passed || fr.declared {
			continue
		}
		if pastObjectDecl(name[len(fr.dir)+1:]) {
			fr.passed = true
			held := fr.held
			fr.held = nil
			if !l.release(i, held) {
				return false
			}
		}
	}
	decl, err := ParseNamaste(path.Base(name))
	if err != nil || !decl.IsObject() {
		return true
	}
	if len(l.frames) == 0 {
		// a declaration in the root directory: it has no ancestors to wait
		// for, and the root isn't an object root, so it never makes
		// declarations below it nested.
		return l.yield(f, nil)
	}
	dirFrame := len(l.frames) - 1
	for _, fr := range l.frames[:dirFrame] {
		if fr.declared {
			return true // nested
		}
	}
	l.frames[dirFrame].declared = true
	l.frames[dirFrame].held = nil // nested
	return l.release(dirFrame, []*ocflfs.FileRef{f})
}

// finish yields the declarations still held at the end of the listing that
// aren't nested.
func (l *declLister) finish() {
	for len(l.frames) > 0 {
		top := l.frames[len(l.frames)-1]
		l.frames = l.frames[:len(l.frames)-1]
		if !top.declared && !l.release(len(l.frames), top.held) {
			return
		}
	}
}

// release hands refs, which are under the directories of all frames before
// frames[i], to the deepest unresolved frame before frames[i], or yields
// them if there is none. It returns false if iteration should stop.
func (l *declLister) release(i int, refs []*ocflfs.FileRef) bool {
	if len(refs) == 0 {
		return true
	}
	for j := i - 1; j >= 0; j-- {
		fr := &l.frames[j]
		if fr.declared {
			return true // nested
		}
		if !fr.passed {
			fr.held = append(fr.held, refs...)
			return true
		}
	}
	for _, ref := range refs {
		if !l.yield(ref, nil) {
			return false
		}
	}
	return true
}

// isUnder reports whether name is under directory dir.
func isUnder(name, dir string) bool {
	return len(name) > len(dir) && name[len(dir)] == '/' && strings.HasPrefix(name, dir)
}

// pastObjectDecl reports whether a file at rest, relative to some directory,
// comes after any object declaration that directory could have, in both
// orders [ocflfs.FileWalker] allows. A declaration is "0=ocfl_object_"
// followed by a version without a "/", so it's past if it sorts after that
// prefix without starting with it. This holds equally for the whole of rest
// (lexical order) and for its first path element (depth-first order), since
// every byte of the prefix sorts after "/".
func pastObjectDecl(rest string) bool {
	return rest > objectDeclPrefix && !strings.HasPrefix(rest, objectDeclPrefix)
}

// listOrder checks that a sequence of paths is strictly increasing in one of
// the orders [ocflfs.FileWalker] allows: full-path lexical order or
// depth-first order with sorted entry names. The two differ for names with
// bytes that sort before "/": "a-b" < "a/c", but a directory "a" and its
// contents come before the sibling "a-b" in a depth-first walk.
type listOrder struct {
	prev       string
	started    bool
	notLexical bool
	notDepth   bool
}

// next checks name against the previous path. It returns an error if the
// paths so far are in neither order.
func (o *listOrder) next(name string) error {
	if o.started {
		if o.prev >= name {
			o.notLexical = true
		}
		if compareByElement(o.prev, name) >= 0 {
			o.notDepth = true
		}
		if o.notLexical && o.notDepth {
			return fmt.Errorf("listing isn't in lexical or depth-first order: %q came after %q", name, o.prev)
		}
	}
	o.prev = name
	o.started = true
	return nil
}

// compareByElement compares paths a and b element by element, as a
// depth-first walk with sorted entry names orders them.
func compareByElement(a, b string) int {
	for {
		aElem, aRest, aMore := strings.Cut(a, "/")
		bElem, bRest, bMore := strings.Cut(b, "/")
		if c := strings.Compare(aElem, bElem); c != 0 {
			return c
		}
		switch {
		case !aMore && !bMore:
			return 0
		case !aMore:
			return -1
		case !bMore:
			return 1
		}
		a, b = aRest, bRest
	}
}

// Objects returns an iterator that yields objects or an error for every object
// declaration file in the root, as yielded by [Root.ObjectDeclarations].
// Objects are yielded in arbitrary order.
func (r *Root) Objects(ctx context.Context, opts ...ObjectOption) iter.Seq2[*Object, error] {
	return r.ObjectsBatch(ctx, 0, opts...)
}

// ObjectsBatch returns an iterator that yields objects or an error for every object
// declaration file in the root, as yielded by [Root.ObjectDeclarations].
// Objects are read in numgos go routines and are yielded in arbitrary order.
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
	opts = append(opts, validationWithRoot(r))
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
			return fmt.Errorf("setting layout extension %q: %w", name, err)
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
		return fmt.Errorf("setting layout extension %q: %w", name, extension.ErrNotLayout)
	}
	if err := layout.Valid(); err != nil {
		return fmt.Errorf("setting layout extension %q: %w", name, err)
	}
	r.layout = layout
	return nil
}

// readLayoutConfig reads the `ocfl_layout.json` files in the storage root
// and unmarshals into the value pointed to by layout
func (r *Root) readLayoutConfig(ctx context.Context) error {
	layoutPath := path.Join(r.dir, layoutConfigFile)
	f, err := r.fs.OpenFile(ctx, layoutPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			r.layoutConfig = nil
			return nil
		}
		return err
	}
	defer func() { _ = f.Close() }()
	layout := map[string]string{}
	if err := json.NewDecoder(f).Decode(&layout); err != nil {
		return fmt.Errorf("storage root layout config is invalid: in %s: %w", layoutPath, err)
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
		return nil, fmt.Errorf("opening extension config: %s: %w", name, err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading extension config: %s: %w", name, err)
	}
	ext, err := extension.DefaultRegistry().Unmarshal(b)
	if err != nil {
		return nil, fmt.Errorf("extension config has errors: in %s: %w", confPath, err)
	}
	return ext, nil
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
