package ocfl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"
)

type Object interface {
	ContentSource
	FixitySource

	DigestAlgorithm() string
	Head() VNum
	ID() string
	Manifest() DigestMap
	Root() *ObjectRoot
	ReadInventory(context.Context) error
	Spec() Spec
	Stage(int) (*Stage, error)
	Version(int) ObjectVersion
	// Validate
	// Commit
}

type Inventory interface {
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

func OpenObject(ctx context.Context, root *ObjectRoot, opts ...func(*ObjectOptions)) (Object, error) {
	objOptions := ObjectOptions{}
	for _, o := range opts {
		o(&objOptions)
	}
	// determine implementation of OCFL spec to use
	var useSpec Spec
	switch {
	case !objOptions.UseSpec.Empty():
		// spec was set explicitly in options
		useSpec = objOptions.UseSpec
	case root.State != nil:
		// use existing state from previous ReadRoot()
		if root.State.Empty() {
			// root directory is empty
			// use a default spec?
		}
		if !root.State.Spec.Empty() {
			// root directory includes an object declaration
			useSpec = root.State.Spec
		}
	default:
		// try to read root to get spec
		err := root.ReadRoot(ctx)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
			// object root directory doesn't exist
			// use default spec?
			return nil, fmt.Errorf("set the OCFL version explicitly to open objects for creation: %w", err)
		}
		if root.State == nil {
			panic("root state wasn't set correctly in ReadRoot")
		}
		if !root.State.HasNamaste() {
			return nil, fmt.Errorf("path is not an OCFL object: %w", ErrObjectNamasteNotExist)
		}
		useSpec = root.State.Spec
	}
	if useSpec.Empty() {
		// FIXME: revert to a default?
		return nil, errors.New("couldn't determine an appropriate OCFL version for openning the object")
	}
	ocflImpl, err := GetOCFL(useSpec)
	if err != nil {
		return nil, fmt.Errorf("openning object with OCFL version %s: %w", useSpec, err)
	}
	return ocflImpl.NewObject(ctx, root, func(opt *ObjectOptions) {
		*opt = objOptions
	})
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
	// ID is the expected ID for the object (if being read), or the new ID for
	// the object (if being created). ID is only required if the object is being
	// created.
	ID string
	// UseSpec sets the OCFL specification that should be used for accessing or
	// creating the object.
	UseSpec Spec

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

func ObjectUseSpec(spec Spec) ObjectOptionsFunc {
	return func(opt *ObjectOptions) {
		opt.UseSpec = spec
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
