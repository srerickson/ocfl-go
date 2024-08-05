package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"time"

	"log/slog"
)

type Object struct {
	reader ReadObject
	opts   objectOptions
}

type ObjectOption func(*objectOptions)

// func ObjectMustExist() ObjectOptionsFunc {
// 	return func(opt *ObjectOptions) {
// 		opt.MustExist = true
// 	}
// }

func ObjectUseOCFL(ocfl OCFL) ObjectOption {
	return func(opt *objectOptions) {
		opt.ocfl = ocfl
	}
}

type objectOptions struct {
	globals Config // global settings
	ocfl    OCFL   // the OCFL implementation used to open the object
}

// NewObject returns an *Object reference for managing the OCFL object at
// path in fsys. The object doesn't need to exist when NewObject is called.
func NewObject(ctx context.Context, fsys FS, path string, opts ...ObjectOption) (*Object, error) {
	if !fs.ValidPath(path) {
		return nil, fmt.Errorf("invalid object path: %q: %w", path, fs.ErrInvalid)
	}
	obj := &Object{}
	for _, optFn := range opts {
		optFn(&obj.opts)
	}
	if obj.opts.ocfl == nil {
		// check for object declaration in object root
		entries, err := fsys.ReadDir(ctx, path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("reading object root contents: %w", err)
			}
		}
		rootState := ParseObjectRootDir(entries)
		switch {
		case rootState.Empty():
			// open as new/uninitialized object w/o an OCFL spec.
			obj.reader = &uninitializedObject{fs: fsys, path: path}
			return obj, nil
		case rootState.HasNamaste():
			obj.opts.ocfl, err = obj.opts.globals.GetSpec(rootState.Spec)
			if err != nil {
				return nil, fmt.Errorf("with OCFL spec found in object root %q: %w", rootState.Spec, err)
			}
		default:
			return nil, fmt.Errorf("directory is not an OCFL object: %w", ErrObjectNamasteNotExist)
		}
	}
	var err error
	obj.reader, err = obj.opts.ocfl.NewReadObject(ctx, fsys, path)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (obj *Object) Commit(ctx context.Context, commit *Commit) error {
	if _, isWriteFS := obj.reader.FS().(WriteFS); !isWriteFS {
		return errors.New("object's backing file system doesn't support write operations")
	}
	var useOCFL OCFL
	switch {
	case commit.Spec.Empty():
		switch {
		case obj.Exists():
			useOCFL = obj.opts.ocfl
		default:
			useOCFL = defaultOCFLs.latest
		}
		commit.Spec = useOCFL.Spec()
	default:
		var err error
		useOCFL, err = obj.opts.globals.GetSpec(commit.Spec)
		if err != nil {
			return err
		}
	}
	newSpecObj, err := useOCFL.Commit(ctx, obj.reader, commit)
	if err != nil {
		return err
	}
	obj.reader = newSpecObj
	if obj.opts.ocfl != useOCFL {
		obj.opts.ocfl = useOCFL
	}
	return nil
}

func (obj *Object) Exists() bool { return ObjectExists(obj.reader) }

// ExtensionNames returns the names of directories in the object's
// extensions directory. The ObjectRoot's State is initialized if it is
// nil. If the object root does not include an object declaration, an error
// is returned. If object root does not include an extensions directory both
// return values are nil.
func (obj Object) ExtensionNames(ctx context.Context) ([]string, error) {
	entries, err := obj.FS().ReadDir(ctx, path.Join(obj.Path(), ExtensionsDir))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			// if the extensions directory includes non-directory
			// entries, should we return an error?
			continue
		}
		names = append(names, e.Name())
	}
	return names, err
}

func (obj *Object) FS() FS {
	return obj.reader.FS()
}

func (obj *Object) Inventory() ReadInventory {
	return obj.reader.Inventory()
}

func (obj *Object) Path() string {
	return obj.reader.Path()
}

// OpenVersion returns an ObjectVersionFS for the version with the given
// index (1...HEAD). If i is 0, the most recent version is used.
func (obj *Object) OpenVersion(ctx context.Context, i int) (*ObjectVersionFS, error) {
	if !obj.Exists() {
		return nil, ErrNamasteNotExist
	}
	inv := obj.Inventory()
	if inv == nil {
		// FIXME; better error
		return nil, errors.New("object is missing an inventory")
	}
	if i == 0 {
		i = inv.Head().num
	}
	ver := inv.Version(i)
	if ver == nil {
		// FIXME; better error
		return nil, errors.New("version not found")
	}
	ioFS := obj.reader.VersionFS(ctx, i)
	if ioFS == nil {
		// FIXME; better error
		return nil, errors.New("version not found")
	}
	vfs := &ObjectVersionFS{
		fsys: ioFS,
		ver:  ver,
		num:  i,
		inv:  inv,
	}
	return vfs, nil
}

func (obj *Object) Validate(ctx context.Context, opts ...ValidationOption) *Validation {
	return obj.reader.Validate(ctx, opts...)
}

type Commit struct {
	ID      string
	Stage   *Stage // required
	Message string // required
	User    User   // required

	// advanced options
	Created         time.Time // time.Now is used, if not set
	Spec            Spec      // OCFL specification version for the new object version
	NewHEAD         int       // enforces new object version number
	AllowUnchanged  bool
	ContentPathFunc RemapFunc

	Logger *slog.Logger
}

// Commit error wraps an error from a commit.
type CommitError struct {
	Err error // The wrapped error

	// Dirty indicates the object may be incomplete or invalid as a result of
	// the error.
	Dirty bool
}

func (c CommitError) Error() string {
	return c.Err.Error()
}

func (c CommitError) Unwrap() error {
	return c.Err
}

// func ObjectSetID(id string) ObjectOption {
// 	return func(opt *Object) {
// 		opt.id = id
// 	}
// }

// func ObjectMustNotExist() ObjectOptionsFunc {
// 	return func(opt *ObjectOptions) {
// 		opt.MustNotExist = true
// 	}
// }

// func ObjectSkipRead() ObjectOptionsFunc {
// 	return func(opt *ObjectOptions) {
// 		opt.SkipRead = true
// 	}
// }

type ObjectVersionFS struct {
	fsys fs.FS
	ver  ObjectVersion
	inv  ReadInventory
	num  int
}

func (vfs *ObjectVersionFS) GetContent(digest string) (FS, string) {
	dm := vfs.State()
	if dm == nil {
		return nil, ""
	}
	pths := dm[digest]
	if len(pths) < 1 {
		return nil, ""
	}
	return &ioFS{FS: vfs.fsys}, pths[0]
}

func (vfs *ObjectVersionFS) Close() error {
	if closer, isCloser := vfs.fsys.(io.Closer); isCloser {
		return closer.Close()
	}
	return nil
}
func (vfs *ObjectVersionFS) Created() time.Time                { return vfs.ver.Created() }
func (vfs *ObjectVersionFS) DigestAlgorithm() string           { return vfs.inv.DigestAlgorithm() }
func (vfs *ObjectVersionFS) State() DigestMap                  { return vfs.ver.State() }
func (vfs *ObjectVersionFS) Message() string                   { return vfs.ver.Message() }
func (vfs *ObjectVersionFS) Num() int                          { return vfs.num }
func (vfs *ObjectVersionFS) Open(name string) (fs.File, error) { return vfs.fsys.Open(name) }
func (vfs *ObjectVersionFS) User() *User                       { return vfs.ver.User() }

func (vfs *ObjectVersionFS) Stage() *Stage {
	return &Stage{
		State:           vfs.State().Clone(),
		DigestAlgorithm: vfs.inv.DigestAlgorithm(),
		FixitySource:    vfs.inv,
		ContentSource:   vfs,
	}
}

func ObjectExists(obj ReadObject) bool {
	if _, isEmpty := obj.(*uninitializedObject); isEmpty {
		return false
	}
	return true
}

// uninitializedObject is an ObjectReader for an object that doesn't exist yet.
type uninitializedObject struct {
	fs   FS
	path string
}

// FS for accessing object contents
func (o *uninitializedObject) FS() FS { return o.fs }

func (o *uninitializedObject) Inventory() ReadInventory { return nil }

// Path returns the object's path relative to its FS()
func (o *uninitializedObject) Path() string { return o.path }

func (o *uninitializedObject) Validate(_ context.Context, _ ...ValidationOption) *Validation {
	result := &Validation{}
	result.AddFatal(fmt.Errorf("empty or missing path: %s: %w", o.path, ErrNamasteNotExist))
	return result
}

// VersionFS returns a value that implements an io/fs.FS for
// accessing the logical contents of the object version state
// with the index v.
func (o *uninitializedObject) VersionFS(ctx context.Context, v int) fs.FS { return nil }

// func (obj *Object) ID() string { return obj.id }

// ValidateNamaste reads and validates the contents of the OCFL object
// declaration in the object root. The ObjectRoot's State is initialized if it
// is nil.
// func (obj *Object) ValidateNamaste(ctx context.Context) error {
// 	decl := Namaste{Type: NamasteTypeObject, Version: obj.ocfl.Spec()}
// 	name := path.Join(obj.Path(), decl.Name())
// 	err := ValidateNamaste(ctx, obj.FS(), name)
// 	if err != nil {
// 		if errors.Is(err, fs.ErrNotExist) {
// 			return fmt.Errorf("%s: %w", name, ErrObjectNamasteNotExist)
// 		}
// 		return err
// 	}
// 	return nil
// }

// UnmarshalInventory unmarshals the inventory.json file in the object root's
// sub-directory, dir, into the value pointed to by v. For example, set dir to
// `v1` to unmarshall the object's v1 inventory. Set dir to `.` to unmarshal the
// root inventory.
// func (obj Object) UnmarshalInventory(ctx context.Context, dir string, v any) (err error) {
// 	name := inventoryFile
// 	if dir != `.` {
// 		name = dir + "/" + name
// 	}
// 	f, err := obj.OpenFile(ctx, name)
// 	if err != nil {
// 		return
// 	}
// 	defer func() {
// 		if closeErr := f.Close(); closeErr != nil {
// 			err = errors.Join(err, f.Close())
// 		}
// 	}()
// 	bytes, err := io.ReadAll(f)
// 	if err != nil {
// 		return
// 	}
// 	err = json.Unmarshal(bytes, v)
// 	return
// }

// // OpenFile opens a file using a name relative to the object root's path
// func (obj *Object) OpenFile(ctx context.Context, name string) (fs.File, error) {
// 	if obj.Path() != "." {
// 		// using path.Join might hide potentially invalid values for
// 		// obj.Path or name.
// 		name = obj.Path() + "/" + name
// 	}
// 	return obj.FS().OpenFile(ctx, name)
// }

// ReadDir reads a directory using a name relative to the object root's dir.
// func (obj *Object) ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error) {
// 	if obj.Path() != "." {
// 		switch {
// 		case name == ".":
// 			name = obj.Path()
// 		default:
// 			name = obj.Path() + "/" + name
// 		}
// 	}
// 	return obj.FS().ReadDir(ctx, name)
// }
