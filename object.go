package ocfl

import (
	"context"
	"fmt"
	"io/fs"
	"time"
)

type Object struct {
	root *ObjectRoot
	// inv    Inventory
	ocfl   OCFL
	id     string
	config Config
}

func (obj *Object) FS() FS { return obj.root.FS }

func (obj *Object) Path() string { return obj.root.Path }

func (obj *Object) ID() string { return obj.id }

func (obj *Object) Exists(ctx context.Context) (bool, error) {
	dirExists, err := obj.root.Exists(ctx)
	if err != nil {
		return false, err
	}
	if !dirExists {
		// object root doesn't exist
		return false, nil
	}
	if obj.root.State.Empty() {
		// object root is an empty directory
		return false, nil
	}
	if obj.root.State.HasNamaste() {
		// object root has an object namaste file
		return true, nil
	}
	return false, fmt.Errorf("object root is not an OCFL object: %w", ErrObjectNamasteNotExist)
}

// OpenVersion returns an ObjectVersionFS for the version with the given
// index (1...HEAD).
func (obj *Object) OpenVersion(ctx context.Context, i int) (ObjectVersionFS, error) {
	return obj.ocfl.OpenVersion(ctx, obj, i)
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
	return useOCFL.Commit(ctx, obj, commit)
}

type Commit struct {
	Stage   *Stage // required
	Message string // required
	User    User   // required

	// advanced options
	Created        time.Time // time.Now is used, if not set
	Upgrade        Spec      // used to upgrade object to newer OCFL
	NewHEAD        int       // enforces new object version number
	AllowUnchanged bool
}

func OpenObject(ctx context.Context, root *ObjectRoot, opts ...func(*Object)) (*Object, error) {
	obj := &Object{root: root}
	for _, optFn := range opts {
		optFn(obj)
	}
	if obj.ocfl == nil {
		rootDirExists, err := obj.root.Exists(ctx)
		if err != nil {
			return nil, fmt.Errorf("accessing object root contents: %w", err)
		}
		ocflRegister := obj.config.OCFLs
		if ocflRegister == nil {
			ocflRegister = &defaultOCFLs
		}
		switch {
		case rootDirExists && root.State.HasNamaste():
			useSpec := root.State.Spec
			ocflImpl, err := ocflRegister.Get(useSpec)
			if err != nil {
				return nil, fmt.Errorf("with OCFL spec found in object root %q: %w", useSpec, err)
			}
			obj.ocfl = ocflImpl
		case !rootDirExists || root.State.Empty():
			ocflImpl, err := ocflRegister.Latest()
			if err != nil {
				return nil, fmt.Errorf("with latest OCFL spec: %w", err)
			}
			obj.ocfl = ocflImpl
		default:
			err = fmt.Errorf("can't identify an OCFL specification for the object: %w", ErrObjectNamasteNotExist)
			return nil, err
		}
	}

	// inv := obj.OCFL.Inventory()
	// if err := obj.Root.UnmarshalInventory(ctx, ".", inv); err != nil {
	// 	return nil, err
	// }
	return obj, nil
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

func ObjectSetID(id string) ObjectOption {
	return func(opt *Object) {
		opt.id = id
	}
}

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
