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

	"github.com/hashicorp/go-multierror"
)

// OpenObject returns a new Object reference for managing the OCFL object at
// path in fs. The object doesn't need to exist when OpenObject is called.
func OpenObject(ctx context.Context, fsys FS, path string, opts ...func(*Object)) (*Object, error) {
	if !fs.ValidPath(path) {
		return nil, fmt.Errorf("invalid object path: %q: %w", path, fs.ErrInvalid)
	}
	obj := &Object{}
	for _, optFn := range opts {
		optFn(obj)
	}
	if obj.ocfl == nil {
		// check for object declaration in object root
		entries, err := fsys.ReadDir(ctx, path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("reading object root contents: %w", err)
			}
		}
		rootState := ParseObjectRootDir(entries)
		ocflRegister := obj.config.OCFLs
		if ocflRegister == nil {
			ocflRegister = &defaultOCFLs
		}
		switch {
		case rootState == nil || rootState.Empty():
			// path doesn't exist or is empty
			obj.specObj = &uninitializedObject{fs: fsys, path: path}
			return obj, nil
		case rootState.HasNamaste():
			obj.ocfl, err = ocflRegister.Get(rootState.Spec)
			if err != nil {
				return nil, fmt.Errorf("with OCFL spec found in object root %q: %w", rootState.Spec, err)
			}
		default:
			return nil, fmt.Errorf("can't identify an OCFL specification for the object: %w", ErrObjectNamasteNotExist)
		}
	}
	var err error
	obj.specObj, err = obj.ocfl.OpenObject(ctx, fsys, path)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

type Object struct {
	specObj SpecObject
	ocfl    OCFL // OCFL Spec used to open/commit specObj
	config  Config
}

func (obj *Object) Commit(ctx context.Context, commit *Commit) error {
	if _, isWriteFS := obj.specObj.FS().(WriteFS); !isWriteFS {
		return errors.New("object's backing file system doesn't support write operations")
	}
	useOCFL := obj.ocfl
	if !commit.Spec.Empty() {
		var err error
		useOCFL, err = obj.config.GetSpec(commit.Spec)
		if err != nil {
			return err
		}
	}
	newSpecObj, err := useOCFL.Commit(ctx, obj.specObj, commit)
	if err != nil {
		return err
	}
	if obj.ocfl != useOCFL {
		obj.ocfl = useOCFL
	}
	obj.specObj = newSpecObj
	return nil
}

func (obj *Object) Close() error { return obj.specObj.Close() }

func (obj *Object) Exists() bool { return obj.specObj.Exists() }

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

func (obj *Object) FS() FS { return obj.specObj.FS() }

func (obj *Object) Inventory() Inventory { return obj.specObj.Inventory() }

func (obj *Object) Path() string { return obj.specObj.Path() }

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

// OpenVersion returns an ObjectVersionFS for the version with the given
// index (1...HEAD).
func (obj *Object) OpenVersion(ctx context.Context, i int) (*ObjectVersionFS, error) {
	if !obj.Exists() {
		return nil, ErrNamasteNotExist
	}
	ver := obj.Inventory().Version(i)
	if ver == nil {
		// FIXME; better error
		return nil, errors.New("version not found")
	}

	vfs := obj.specObj.VersionFS(ctx, i)
	if vfs == nil {
		// FIXME; better error
		return nil, errors.New("version not found")
	}
	return &ObjectVersionFS{fsys: vfs, ver: ver}, nil
}

func (obj *Object) Validate(ctx context.Context, opts *Validation) *ValidationResult {
	return obj.specObj.Validate(ctx, opts)
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

type ObjectOption func(*Object)

// func ObjectMustExist() ObjectOptionsFunc {
// 	return func(opt *ObjectOptions) {
// 		opt.MustExist = true
// 	}
// }

func ObjectUseOCFL(ocfl OCFL) ObjectOption {
	return func(opt *Object) {
		opt.ocfl = ocfl
	}
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

type SpecObject interface {
	// Close closes the object, freeing allocated resources.
	Close() error
	// Exists return bool indicating if the object exists or not
	Exists() bool
	// FS for accessing object contents
	FS() FS
	Inventory() Inventory
	// Path returns the object's path relative to its FS()
	Path() string
	Validate(context.Context, *Validation) *ValidationResult
	// VersionFS returns a value that implements an io/fs.FS for
	// accessing the logical contents of the object version state
	// with the index v.
	VersionFS(ctx context.Context, v int) FSCloser
}

type Inventory interface {
	FixitySource
	ContentDirectory() string
	DigestAlgorithm() string
	Head() VNum
	ID() string
	Manifest() DigestMap
	Spec() Spec
	Version(int) ObjectVersion
}

type ObjectVersion interface {
	State() DigestMap
	User() *User
	Message() string
	Created() time.Time
}

type ObjectVersionFS struct {
	fsys FSCloser
	ver  ObjectVersion
}

func (vfs *ObjectVersionFS) Open(name string) (fs.File, error) { return vfs.fsys.Open(name) }
func (vfs *ObjectVersionFS) Close() error                      { return vfs.fsys.Close() }
func (vfs *ObjectVersionFS) State() DigestMap                  { return vfs.ver.State() }
func (vfs *ObjectVersionFS) Message() string                   { return vfs.ver.Message() }
func (vfs *ObjectVersionFS) User() *User                       { return vfs.ver.User() }
func (vfs *ObjectVersionFS) Created() time.Time                { return vfs.ver.Created() }

// User is a generic user information struct
type User struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}

type FSCloser interface {
	fs.FS
	io.Closer
}

type uninitializedObject struct {
	fs   FS
	path string
}

// Close closes the object, freeing allocated resources.
func (o *uninitializedObject) Close() error { return nil }

// Exists return bool indicating if the object exists or not
func (o *uninitializedObject) Exists() bool {
	return false
}

// FS for accessing object contents
func (o *uninitializedObject) FS() FS {
	return o.fs
}

func (o *uninitializedObject) Inventory() Inventory {
	return nil
}

// Path returns the object's path relative to its FS()
func (o *uninitializedObject) Path() string {
	return o.path
}

func (o *uninitializedObject) Validate(_ context.Context, _ *Validation) *ValidationResult {
	result := &ValidationResult{Fatal: &multierror.Error{}}
	result.Fatal = multierror.Append(result.Fatal, fmt.Errorf("empty or missing path: %s: %w", o.path, ErrNamasteNotExist))
	return result
}

// VersionFS returns a value that implements an io/fs.FS for
// accessing the logical contents of the object version state
// with the index v.
func (o *uninitializedObject) VersionFS(ctx context.Context, v int) FSCloser {
	return nil
}
