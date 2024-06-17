package ocfl

import (
	"context"
	"fmt"
)

type Object interface {
	Exists(context.Context) (bool, error)
	Root() *ObjectRoot
}

// type Inventory interface {
// 	DigestAlgorithm() string
// 	Head() VNum
// 	ID() string
// 	Manifest() DigestMap
// 	Spec() Spec
// 	Version(int) ObjectVersion
// }

// type ObjectVersion interface {
// 	State() DigestMap
// 	User() *User
// 	Message() string
// 	Created() time.Time
// }

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
	if objOptions.Config.OCFLs == nil {
		objOptions.Config.OCFLs = &defaultOCFLs
	}
	withOptions := func(opt *ObjectOptions) { *opt = objOptions }
	// check if the OCFL spec was set explicitly
	if useSpec := objOptions.UseSpec; !useSpec.Empty() {
		ocflImpl, err := objOptions.Config.OCFLs.Get(useSpec)
		if err != nil {
			return nil, fmt.Errorf("with explicit OCFL spec %q: %w", useSpec, err)
		}
		return ocflImpl.NewObject(ctx, root, withOptions)
	}
	// Use the OCFL spec found in object root, if present
	rootDirExists, err := root.Exists(ctx)
	if err != nil {
		return nil, fmt.Errorf("accessing object root contents: %w", err)
	}
	if rootDirExists && root.State.HasNamaste() {
		useSpec := root.State.Spec
		ocflImpl, err := objOptions.Config.OCFLs.Get(useSpec)
		if err != nil {
			return nil, fmt.Errorf("with OCFL spec found in object root %q: %w", useSpec, err)
		}
		return ocflImpl.NewObject(ctx, root, withOptions)
	}
	// Use the latest OCFL if object root is missing or empty
	if !rootDirExists || root.State.Empty() {
		ocflImpl, err := objOptions.Config.OCFLs.Latest()
		if err != nil {
			return nil, fmt.Errorf("with latest OCFL spec: %w", err)
		}
		return ocflImpl.NewObject(ctx, root, withOptions)
	}
	// give, up because there is no OCFL object declaration
	return nil, fmt.Errorf("can't identify an OCFL specification for the object: %w", ErrObjectNamasteNotExist)
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
