package ocfl

import (
	"context"
	"errors"
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

func NewObject(ctx context.Context, root *ObjectRoot, opts ...func(*ObjectOptions)) (Object, error) {
	var ocflImpl OCFL // OCFL spec implementation
	objOptions := ObjectOptions{}
	for _, o := range opts {
		o(&objOptions)
	}
	// determine implementation of OCFL spec to use
	switch {
	case !objOptions.UseSpec.Empty():
		// ocfl spect set explicitly in options
		ocflImpl = GetOCFL(objOptions.UseSpec)
	case !objOptions.SkipRead:
		// get spec by reading the object root directory
		err := root.ReadRoot(ctx)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
			// object doesn't exist
			if objOptions.MustExist {
				return nil, err
			}
		}
		if err == nil && objOptions.MustNotExist {
			return nil, ErrObjectNamasteExists
		}
		if root.State != nil {
			ocflImpl = GetOCFL(root.State.Spec)
		}
	default:
		return nil, errors.New("NewObject options prevented determining the OCFL implementation for the object")
	}
	if ocflImpl == nil {
		return nil, errors.New("could not determine OCFL implementation for the object")
	}
	return ocflImpl.NewObject(ctx, root, func(opt *ObjectOptions) {
		*opt = objOptions
	})
}

type ObjectOptions struct {
	// ID is the expected ID for the object (if being read), or the new ID for
	// the object (if being created)
	ID string
	// UseSpec sets the OCFL specification that should be used for accessing or
	// creating the object.
	UseSpec Spec
	// MustExist: if the object doesn't exist, return an error
	MustExist bool
	// MustNotExist: if the object does exist, return an error
	MustNotExist bool
	// SkipRead prevents the object from being accessed during NewObject(). If
	// set, MustExist and MustNotExist are ignored.
	SkipRead bool
}

type ObjectOptionsFunc func(*ObjectOptions)

func ObjectMustExist() ObjectOptionsFunc {
	return func(opt *ObjectOptions) {
		opt.MustExist = true
	}
}

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

func ObjectMustNotExist() ObjectOptionsFunc {
	return func(opt *ObjectOptions) {
		opt.MustNotExist = true
	}
}

func ObjectSkipRead() ObjectOptionsFunc {
	return func(opt *ObjectOptions) {
		opt.SkipRead = true
	}
}
