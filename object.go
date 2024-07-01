package ocfl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"time"
)

// OpenObject returns a new Object reference for managing the OCFL object at
// root. The object doesn't need to exist when OpenObject is called.
func OpenObject(ctx context.Context, fsys FS, path string, opts ...func(*Object)) (*Object, error) {
	if !fs.ValidPath(path) {
		return nil, fmt.Errorf("invalid object path: %q: %w", path, fs.ErrInvalid)
	}
	obj := &Object{fs: fsys, path: path}
	for _, optFn := range opts {
		optFn(obj)
	}
	if obj.ocfl == nil {
		rootState, err := obj.RootState(ctx)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("reading object root contents: %w", err)
			}
		}
		ocflRegister := obj.config.OCFLs
		if ocflRegister == nil {
			ocflRegister = &defaultOCFLs
		}
		switch {
		case rootState == nil || rootState.Empty():
			ocflImpl, err := ocflRegister.Latest()
			if err != nil {
				return nil, fmt.Errorf("with latest OCFL spec: %w", err)
			}
			obj.ocfl = ocflImpl
		case rootState.HasNamaste():
			ocflImpl, err := ocflRegister.Get(rootState.Spec)
			if err != nil {
				return nil, fmt.Errorf("with OCFL spec found in object root %q: %w", rootState.Spec, err)
			}
			obj.ocfl = ocflImpl

		default:
			return nil, fmt.Errorf("can't identify an OCFL specification for the object: %w", ErrObjectNamasteNotExist)
		}
	}
	return obj, nil
}

type Object struct {
	fs     FS
	path   string
	ocfl   OCFL
	config Config
}

func (obj *Object) FS() FS { return obj.fs }

func (obj *Object) Path() string { return obj.path }

// func (obj *Object) ID() string { return obj.id }

// ValidateNamaste reads and validates the contents of the OCFL object
// declaration in the object root. The ObjectRoot's State is initialized if it
// is nil.
func (obj *Object) ValidateNamaste(ctx context.Context) error {
	decl := Namaste{Type: NamasteTypeObject, Version: obj.ocfl.Spec()}
	name := path.Join(obj.path, decl.Name())
	err := ValidateNamaste(ctx, obj.fs, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: %w", name, ErrObjectNamasteNotExist)
		}
		return err
	}
	return nil
}

func (obj Object) RootState(ctx context.Context) (*ObjectRootState, error) {
	entries, err := obj.ReadDir(ctx, ".")
	if err != nil {
		return nil, err
	}
	return ParseObjectRootDir(entries), nil
}

// ExtensionNames returns the names of directories in the object's
// extensions directory. The ObjectRoot's State is initialized if it is
// nil. If the object root does not include an object declaration, an error
// is returned. If object root does not include an extensions directory both
// return values are nil.
func (obj Object) ExtensionNames(ctx context.Context) ([]string, error) {
	entries, err := obj.ReadDir(ctx, ExtensionsDir)
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

// UnmarshalInventory unmarshals the inventory.json file in the object root's
// sub-directory, dir, into the value pointed to by v. For example, set dir to
// `v1` to unmarshall the object's v1 inventory. Set dir to `.` to unmarshal the
// root inventory.
func (obj Object) UnmarshalInventory(ctx context.Context, dir string, v any) (err error) {
	name := inventoryFile
	if dir != `.` {
		name = dir + "/" + name
	}
	f, err := obj.OpenFile(ctx, name)
	if err != nil {
		return
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, f.Close())
		}
	}()
	bytes, err := io.ReadAll(f)
	if err != nil {
		return
	}
	err = json.Unmarshal(bytes, v)
	return
}

// OpenFile opens a file using a name relative to the object root's path
func (obj *Object) OpenFile(ctx context.Context, name string) (fs.File, error) {
	if obj.path != "." {
		// using path.Join might hide potentially invalid values for
		// obj.Path or name.
		name = obj.path + "/" + name
	}
	return obj.fs.OpenFile(ctx, name)
}

// ReadDir reads a directory using a name relative to the object root's dir.
func (obj *Object) ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error) {
	if obj.path != "." {
		switch {
		case name == ".":
			name = obj.path
		default:
			name = obj.path + "/" + name
		}
	}
	return obj.fs.ReadDir(ctx, name)
}

// OpenVersion returns an ObjectVersionFS for the version with the given
// index (1...HEAD).
func (obj *Object) OpenVersion(ctx context.Context, i int) (ObjectVersionFS, error) {
	//return obj.ocfl.OpenVersion(ctx, obj, i)
	return nil, errors.New("not implemented")
}

func (obj *Object) Commit(ctx context.Context, commit *Commit) error {
	useOCFL := obj.ocfl
	if !commit.Upgrade.Empty() {
		var err error
		useOCFL, err = obj.config.GetSpec(commit.Upgrade)
		if err != nil {
			return err
		}
	}
	writeFS, ok := obj.FS().(WriteFS)
	if !ok {
		return errors.New("object's backing file system doesn't support writes")
	}
	_, err := useOCFL.Commit(ctx, writeFS, obj.path, commit)
	if err != nil {
		return err
	}
	return nil
}

type Commit struct {
	ID      string
	Stage   *Stage // required
	Message string // required
	User    User   // required

	// advanced options
	Created        time.Time // time.Now is used, if not set
	Upgrade        Spec      // used to upgrade object to newer OCFL
	NewHEAD        int       // enforces new object version number
	AllowUnchanged bool
}

// type ObjectMode uint8

// const (
// 	ObjectModeReadOnly ObjectMode = iota
// 	// ObjectModeReadWrite
// 	// ObjectModeCreate // When writing, create a new object if neccessary.
// 	// ObjectModeUpdate
// )

// // ObjectOptions are options used in OpenObject
// type ObjectOptions struct {
// 	// Global OCFL Configuration
// 	Config Config
// 	// ID is the expected ID for the object (if being read), or the new ID for
// 	// the object (if being created). ID is only required if the object is being
// 	// created.
// 	ID string
// 	// OCFL sets the OCFL specification that should be used for accessing or
// 	// creating the object.
// 	OCFL OCFL

// 	// MustExist: if the object doesn't exist, return an error
// 	// MustExist bool
// 	// // MustNotExist: if the object does exist, return an error
// 	// MustNotExist bool

// 	// SkipRead prevents the object from being accessed during OpenObject(). If
// 	// set, MustExist is ignored
// 	// SkipRead bool

// 	// Mode used to open the object, determines how the object can be used.
// 	// Mode ObjectMode
// }

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
	Inventory() Inventory
}

type Inventory interface {
	FixitySource
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

// User is a generic user information struct
type User struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}

type ObjectVersionFS interface {
	ObjectVersion
	OpenFile(ctx context.Context, name string) (fs.File, error)
	Close() error
}
