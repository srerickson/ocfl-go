package ocfl

import (
	"context"
	"fmt"
	"time"
)

type Object struct {
	OCFL      OCFL
	Root      *ObjectRoot
	Inventory Inventory
}

func (obj *Object) Exists(ctx context.Context) (bool, error) {
	dirExists, err := obj.Root.Exists(ctx)
	if err != nil {
		return false, err
	}
	if !dirExists {
		// object root doesn't exist
		return false, nil
	}
	if obj.Root.State.Empty() {
		// object root is an empty directory
		return false, nil
	}
	if obj.Root.State.HasNamaste() {
		// object root has an object namaste file
		return true, nil
	}
	return false, fmt.Errorf("object root is not an OCFL object: %w", ErrObjectNamasteNotExist)
}

func OpenObject(ctx context.Context, root *ObjectRoot, opts ...func(*ObjectOptions)) (*Object, error) {
	objOpts := ObjectOptions{}
	for _, optFn := range opts {
		optFn(&objOpts)
	}
	obj := &Object{Root: root, OCFL: objOpts.OCFL}
	if obj.OCFL == nil {
		rootDirExists, err := root.Exists(ctx)
		if err != nil {
			return nil, fmt.Errorf("accessing object root contents: %w", err)
		}
		ocfls := objOpts.Config.OCFLs
		if ocfls == nil {
			ocfls = &defaultOCFLs
		}
		switch {
		case rootDirExists && root.State.HasNamaste():
			useSpec := root.State.Spec
			ocflImpl, err := ocfls.Get(useSpec)
			if err != nil {
				return nil, fmt.Errorf("with OCFL spec found in object root %q: %w", useSpec, err)
			}
			obj.OCFL = ocflImpl
		case !rootDirExists || root.State.Empty():
			ocflImpl, err := ocfls.Latest()
			if err != nil {
				return nil, fmt.Errorf("with latest OCFL spec: %w", err)
			}
			obj.OCFL = ocflImpl
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

type ObjectMode uint8

const (
	ObjectModeReadOnly ObjectMode = iota
	// ObjectModeReadWrite
	// ObjectModeCreate // When writing, create a new object if neccessary.
	// ObjectModeUpdate
)

// ObjectOptions are options used in OpenObject
type ObjectOptions struct {
	// Global OCFL Configuration
	Config Config
	// ID is the expected ID for the object (if being read), or the new ID for
	// the object (if being created). ID is only required if the object is being
	// created.
	ID string
	// OCFL sets the OCFL specification that should be used for accessing or
	// creating the object.
	OCFL OCFL

	// MustExist: if the object doesn't exist, return an error
	// MustExist bool
	// // MustNotExist: if the object does exist, return an error
	// MustNotExist bool

	// SkipRead prevents the object from being accessed during OpenObject(). If
	// set, MustExist is ignored
	// SkipRead bool

	// Mode used to open the object, determines how the object can be used.
	// Mode ObjectMode
}

type ObjectOptionsFunc func(*ObjectOptions)

// func ObjectMustExist() ObjectOptionsFunc {
// 	return func(opt *ObjectOptions) {
// 		opt.MustExist = true
// 	}
// }

func ObjectUseOCFL(ocfl OCFL) ObjectOptionsFunc {
	return func(opt *ObjectOptions) {
		opt.OCFL = ocfl
	}
}

func ObjectSetID(id string) ObjectOptionsFunc {
	return func(opt *ObjectOptions) {
		opt.ID = id
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
